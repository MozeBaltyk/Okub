// Package helper safely orchestrates the maintained Fedora network-services VM.
package helper

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mozebaltyk/Okub/internal/domain"
	"go.yaml.in/yaml/v3"
	"golang.org/x/sys/unix"
)

type Runner interface {
	Run(context.Context, string, []string, []string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, directory string, env, args []string) ([]byte, error) {
	if len(args) == 0 {
		return nil, errors.New("empty command")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = directory
	cmd.Env = mergeEnvironment(os.Environ(), env)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
	}
	cmd.WaitDelay = 5 * time.Second
	var output tailBuffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	if ctx.Err() != nil {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return output.Bytes(), ctx.Err()
	}
	return output.Bytes(), err
}

func mergeEnvironment(base, overrides []string) []string {
	names := map[string]bool{}
	for _, value := range overrides {
		name, _, _ := strings.Cut(value, "=")
		names[name] = true
	}
	result := make([]string, 0, len(base)+len(overrides))
	for _, value := range base {
		name, _, _ := strings.Cut(value, "=")
		if !names[name] && !strings.HasPrefix(name, "TF_") && !strings.HasPrefix(name, "TOFU_") {
			result = append(result, value)
		}
	}
	return append(result, overrides...)
}

const maxCommandOutput = 16 << 20

type tailBuffer struct{ bytes.Buffer }

func (b *tailBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) >= maxCommandOutput {
		b.Reset()
		_, _ = b.Buffer.Write(p[len(p)-maxCommandOutput:])
		return n, nil
	}
	if excess := b.Len() + len(p) - maxCommandOutput; excess > 0 {
		_ = b.Next(excess)
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

type Manager struct {
	Runner          Runner
	LookPath        func(string) (string, error)
	InterfaceCIDRs  func() ([]InterfaceCIDR, error)
	PollInterval    time.Duration
	ShutdownTimeout time.Duration
}

type InterfaceCIDR struct {
	Name   string
	Prefix netip.Prefix
}
type Result struct {
	Operation string `json:"operation"`
	Planned   bool   `json:"planned"`
	Applied   bool   `json:"applied"`
	Changed   bool   `json:"changed"`
	Message   string `json:"message"`
	LogPath   string `json:"logPath"`
}

func (m Manager) Run(ctx context.Context, repoRoot string, definition domain.ClusterDefinition, operation string, apply bool) (Result, error) {
	if definition.Helper == nil {
		return Result{}, errors.New("fedora helper configuration is required")
	}
	p := *definition.Helper
	p.WANMAC = strings.ToLower(p.WANMAC)
	p.LANMAC = strings.ToLower(p.LANMAC)
	if p.LibvirtURI != "qemu:///system" {
		return Result{}, errors.New("Fedora helper lifecycle requires local qemu:///system libvirt")
	}
	if operation != "up" && operation != "down" {
		return Result{}, errors.New("invalid Fedora helper operation")
	}
	if m.Runner == nil {
		m.Runner = ExecRunner{}
	}
	if m.LookPath == nil {
		m.LookPath = exec.LookPath
	}
	if m.InterfaceCIDRs == nil {
		m.InterfaceCIDRs = activeInterfaceCIDRs
	}
	if m.PollInterval <= 0 {
		m.PollInterval = time.Second
	}
	if m.ShutdownTimeout <= 0 {
		m.ShutdownTimeout = 2 * time.Minute
	}
	module := filepath.Join(repoRoot, "lab", "fedora-helper")
	runtime := filepath.Join(repoRoot, ".local", "fedora-helper")
	if err := ensurePrivatePath(repoRoot, ".local", "fedora-helper"); err != nil {
		return Result{}, err
	}
	if err := ensurePrivatePath(repoRoot, ".local", "fedora-helper", "tofu"); err != nil {
		return Result{}, err
	}
	state := filepath.Join(runtime, "terraform.tfstate")
	logPath := filepath.Join(runtime, operation+".log")
	if err := secureManagedRuntimeFile(state); err != nil {
		return Result{}, err
	}
	if err := secureOptionalRuntimeFile(logPath); err != nil {
		return Result{}, err
	}
	lock, err := acquireOperationLock(filepath.Join(runtime, "operation.lock"))
	if err != nil {
		return Result{}, err
	}
	defer lock.Close()

	tofu, err := m.LookPath("tofu")
	if err != nil {
		return Result{}, errors.New("tofu is unavailable")
	}
	virsh, err := m.LookPath("virsh")
	if err != nil {
		return Result{}, errors.New("virsh is unavailable; install libvirt clients")
	}
	var image ImageResult
	cloudDir := filepath.Join(runtime, "cloud-init")
	cloudPaths := []string{filepath.Join(cloudDir, "user-data.yaml"), filepath.Join(cloudDir, "meta-data.yaml"), filepath.Join(cloudDir, "network-config.yaml")}
	if operation == "up" {
		qemuImg, lookupErr := m.LookPath("qemu-img")
		if lookupErr != nil {
			return Result{}, errors.New("qemu-img is unavailable")
		}
		image, err = (ImageSync{}).Sync(ctx, repoRoot, true)
		if err != nil {
			return Result{}, fmt.Errorf("verified Fedora image is required; run `okub helper image sync`: %w", err)
		}
		for _, path := range cloudPaths {
			if err := secureInput(path, repoRoot); err != nil {
				return Result{}, err
			}
		}
		if err := validateCloudInputs(cloudPaths, p, definition); err != nil {
			return Result{}, err
		}
		imageInfo, infoErr := m.Runner.Run(ctx, module, nil, []string{qemuImg, "info", "--output=json", image.ImagePath})
		var qi struct {
			Format      string `json:"format"`
			VirtualSize int64  `json:"virtual-size"`
		}
		if infoErr != nil || json.Unmarshal(imageInfo, &qi) != nil || qi.Format != "qcow2" || qi.VirtualSize <= 0 || qi.VirtualSize > int64(p.RootDiskSizeGiB)<<30 {
			return Result{}, errors.New("Fedora image must be qcow2 and fit the configured root disk")
		}
	}
	env := []string{"TF_IN_AUTOMATION=1", "TF_DATA_DIR=" + filepath.Join(runtime, "tofu")}
	var log tailBuffer
	runWithContext := func(commandCtx context.Context, args ...string) ([]byte, error) {
		if err := commandCtx.Err(); err != nil {
			return nil, err
		}
		out, e := m.Runner.Run(commandCtx, module, env, args)
		_, _ = log.Write(out)
		if ctxErr := commandCtx.Err(); ctxErr != nil {
			return out, ctxErr
		}
		return out, e
	}
	run := func(args ...string) ([]byte, error) { return runWithContext(ctx, args...) }
	inspect := func(summary string, args ...string) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out, err := m.Runner.Run(ctx, module, env, args)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return out, ctxErr
		}
		if err == nil {
			_, _ = log.Write([]byte(summary + "\n"))
		}
		return out, err
	}
	uriOutput, err := run(virsh, "-c", p.LibvirtURI, "uri")
	if err != nil {
		return Result{}, m.finish(runtime, operation, p, log.Bytes(), commandFailure(ctx, err, "local qemu:///system libvirt is unavailable"))
	}
	if strings.TrimSpace(string(uriOutput)) != p.LibvirtURI {
		return Result{}, m.finish(runtime, operation, p, log.Bytes(), errors.New("local qemu:///system libvirt is unavailable"))
	}
	poolOutput, err := run(virsh, "-c", p.LibvirtURI, "pool-info", "--pool", p.StoragePool)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Result{}, m.finish(runtime, operation, p, log.Bytes(), commandFailure(ctx, err, "cannot inspect libvirt storage pool"))
		}
		return Result{}, m.finish(runtime, operation, p, log.Bytes(), fmt.Errorf("libvirt storage pool %q is unavailable; create or start it, then retry", p.StoragePool))
	}
	if !poolIsActive(poolOutput) {
		return Result{}, m.finish(runtime, operation, p, log.Bytes(), fmt.Errorf("libvirt storage pool %q is not active; start it, then retry", p.StoragePool))
	}
	if _, err = run(tofu, "init", "-input=false", "-backend-config=path="+state); err != nil {
		return Result{}, m.finish(runtime, operation, p, log.Bytes(), commandFailure(ctx, err, "tofu init failed"))
	}
	stateOutput, stateErr := run(tofu, "state", "list")
	if stateErr != nil {
		if !isMissingInitialState(state, stateOutput) {
			return Result{}, m.finish(runtime, operation, p, log.Bytes(), commandFailure(ctx, stateErr, "cannot inspect Fedora helper state"))
		}
		stateOutput = nil
	}
	if err := secureManagedRuntimeFile(state); err != nil {
		return Result{}, m.finish(runtime, operation, p, log.Bytes(), err)
	}
	present, stateErr := ownedState(stateOutput)
	if stateErr != nil {
		return Result{}, m.finish(runtime, operation, p, log.Bytes(), stateErr)
	}
	complete := len(present) == len(ownedAddresses)
	if operation == "down" && len(present) != 0 && !complete {
		return Result{}, m.finish(runtime, operation, p, log.Bytes(), errors.New("Fedora helper state is partial; down requires all six owned resources"))
	}
	firstUp := len(present) == 0
	if operation == "down" && firstUp {
		result := Result{Operation: operation, Planned: true, Message: "already down", LogPath: filepath.Join(runtime, operation+".log")}
		return result, m.finish(runtime, operation, p, log.Bytes(), nil)
	}
	if operation == "down" {
		stateJSON, showErr := inspect("state identity inspected", tofu, "show", "-json")
		if showErr != nil {
			return Result{}, m.finish(runtime, operation, p, log.Bytes(), commandFailure(ctx, showErr, "cannot inspect Fedora helper state identity"))
		}
		identity, identityErr := stateDomainIdentity(stateJSON, p.VMName)
		if identityErr != nil {
			return Result{}, m.finish(runtime, operation, p, log.Bytes(), identityErr)
		}
		uuidOutput, uuidErr := run(virsh, "-c", "qemu:///system", "domuuid", p.VMName)
		if uuidErr != nil {
			return Result{}, m.finish(runtime, operation, p, log.Bytes(), commandFailure(ctx, uuidErr, "cannot verify Fedora helper domain identity"))
		}
		if strings.ToLower(strings.TrimSpace(string(uuidOutput))) != identity.UUID {
			return Result{}, m.finish(runtime, operation, p, log.Bytes(), errors.New("Fedora helper domain UUID does not match owned OpenTofu state"))
		}
		result, downErr := m.down(ctx, runWithContext, p, virsh, apply)
		result.LogPath = filepath.Join(runtime, operation+".log")
		if downErr != nil {
			return Result{}, m.finish(runtime, operation, p, log.Bytes(), downErr)
		}
		return result, m.finish(runtime, operation, p, log.Bytes(), nil)
	}
	wanBridge, lanBridge := bridgeNames(p.VMName)
	interfaces, err := m.InterfaceCIDRs()
	if err != nil {
		return Result{}, m.finish(runtime, operation, p, log.Bytes(), errors.New("cannot enumerate active host network CIDRs"))
	}
	excludedValues, err := validateHostNetworks(interfaces, present, wanBridge, lanBridge, p.WANCIDR, definition.Network.MachineCIDR)
	if err != nil {
		return Result{}, m.finish(runtime, operation, p, log.Bytes(), err)
	}
	plan, err := reservePlanPath(runtime, operation)
	if err != nil {
		return Result{}, m.finish(runtime, operation, p, log.Bytes(), err)
	}
	defer os.Remove(plan)
	args := []string{tofu, "plan", "-input=false", "-lock-timeout=30s", "-target=libvirt_domain.helper", "-out=" + plan}
	excludedJSON, _ := json.Marshal(excludedValues)
	vars := map[string]string{"vm_name": p.VMName, "storage_pool": p.StoragePool, "base_image_path": image.ImagePath, "base_image_sha256": image.SHA256, "cloud_init_user_data_path": cloudPaths[0], "cloud_init_meta_data_path": cloudPaths[1], "cloud_init_network_config_path": cloudPaths[2], "wan_cidr": p.WANCIDR, "lan_cidr": definition.Network.MachineCIDR, "cluster_cidr": definition.Network.ClusterCIDR, "service_cidr": definition.Network.ServiceCIDR, "wan_excluded_cidrs": string(excludedJSON), "wan_bridge_name": wanBridge, "lan_bridge_name": lanBridge, "wan_mac": p.WANMAC, "lan_mac": p.LANMAC, "libvirt_uri": p.LibvirtURI, "vcpu": strconv.Itoa(p.VCPU), "memory_mib": strconv.Itoa(p.MemoryMiB), "root_disk_size_gib": strconv.Itoa(p.RootDiskSizeGiB), "wan_network_name": p.VMName + "-wan", "lan_network_name": p.VMName + "-lan", "base_image_volume_name": p.VMName + "-fedora-44.qcow2", "root_volume_name": p.VMName + "-root.qcow2", "seed_volume_name": p.VMName + "-seed.iso"}
	for _, name := range []string{"vm_name", "storage_pool", "base_image_path", "base_image_sha256", "cloud_init_user_data_path", "cloud_init_meta_data_path", "cloud_init_network_config_path", "wan_cidr", "lan_cidr", "cluster_cidr", "service_cidr", "wan_excluded_cidrs", "wan_bridge_name", "lan_bridge_name", "wan_mac", "lan_mac", "libvirt_uri", "vcpu", "memory_mib", "root_disk_size_gib", "wan_network_name", "lan_network_name", "base_image_volume_name", "root_volume_name", "seed_volume_name"} {
		args = append(args, "-var="+name+"="+vars[name])
	}
	if _, err = run(args...); err != nil {
		return Result{}, m.finish(runtime, operation, p, log.Bytes(), commandFailure(ctx, err, "tofu plan failed"))
	}
	if err := securePlan(plan); err != nil {
		return Result{}, m.finish(runtime, operation, p, log.Bytes(), err)
	}
	shown, err := inspect("saved plan inspected", tofu, "show", "-json", plan)
	if err != nil {
		return Result{}, m.finish(runtime, operation, p, log.Bytes(), commandFailure(ctx, err, "tofu show failed"))
	}
	changed, err := validatePlan(shown, present)
	if err != nil {
		return Result{}, m.finish(runtime, operation, p, log.Bytes(), err)
	}
	result := Result{Operation: operation, Planned: true, Changed: changed, Message: "plan is safe"}
	missing := missingOwnedAddresses(present)
	if len(present) > 0 && !complete {
		result.Message = "safe plan will resume creation of: " + strings.Join(missing, ", ")
	}
	if !changed {
		result.Message = "already " + operation
		result.LogPath = filepath.Join(runtime, operation+".log")
		return result, m.finish(runtime, operation, p, log.Bytes(), nil)
	}
	if apply {
		if _, err = run(tofu, "apply", "-input=false", "-lock-timeout=30s", "-auto-approve", plan); err != nil {
			return Result{}, m.finish(runtime, operation, p, log.Bytes(), commandFailure(ctx, err, "tofu apply failed"))
		}
		if err := secureManagedRuntimeFile(state); err != nil {
			return Result{}, m.finish(runtime, operation, p, log.Bytes(), err)
		}
		if err := secureManagedRuntimeFile(state + ".backup"); err != nil {
			return Result{}, m.finish(runtime, operation, p, log.Bytes(), err)
		}
		result.Applied = true
		result.Message = "safe plan applied"
		if firstUp {
			result.Message = "created Fedora base, root and NoCloud volumes, dedicated WAN/LAN networks, and running helper VM; host LAN uses the final usable address"
		} else if !complete {
			result.Message = "resumed creation of: " + strings.Join(missing, ", ")
		}
	} else {
		result.Message = "safe plan created; pass --apply to authorize host mutation"
		if firstUp {
			result.Message = "safe first-up plan creates dedicated WAN/LAN networks, Fedora base, root and NoCloud volumes, and running VM; --apply adds host interfaces/routes"
		} else if !complete {
			result.Message = "safe plan will resume creation of: " + strings.Join(missing, ", ") + "; pass --apply to authorize host mutation"
		}
	}
	result.LogPath = filepath.Join(runtime, operation+".log")
	return result, m.finish(runtime, operation, p, log.Bytes(), nil)
}

func isMissingInitialState(path string, output []byte) bool {
	_, err := os.Lstat(path)
	return errors.Is(err, os.ErrNotExist) && bytes.Contains(output, []byte("No state file was found"))
}

func poolIsActive(output []byte) bool {
	for _, line := range strings.Split(string(output), "\n") {
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "State") {
			return strings.EqualFold(strings.TrimSpace(value), "running")
		}
	}
	return false
}

func (m Manager) down(ctx context.Context, run func(context.Context, ...string) ([]byte, error), p domain.Helper, virsh string, apply bool) (Result, error) {
	downCtx, cancel := context.WithTimeout(ctx, m.ShutdownTimeout)
	defer cancel()
	domstate := func(commandCtx context.Context) (string, error) {
		output, err := run(commandCtx, virsh, "-c", "qemu:///system", "domstate", "--domain", p.VMName)
		if err != nil {
			return "", commandFailure(commandCtx, err, "cannot inspect Fedora helper domain power state")
		}
		return normalizeDomainState(output)
	}
	state, err := domstate(downCtx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return Result{}, errors.New("Fedora helper domain state inspection timed out")
		}
		return Result{}, err
	}
	result := Result{Operation: "down", Planned: true}
	if state == "off" {
		result.Message = "already down; networks, volumes, domain definition, and state retained"
		return result, nil
	}
	result.Changed = true
	if !apply {
		result.Message = "Fedora helper shutdown is needed; pass --apply for graceful virsh shutdown (networks and state are retained)"
		return result, nil
	}
	if state == "running" {
		if output, err := run(downCtx, virsh, "-c", "qemu:///system", "shutdown", "--domain", p.VMName); err != nil {
			if shutdownAlreadyStopped(output) {
				after, stateErr := domstate(downCtx)
				if stateErr == nil && after == "off" {
					result.Applied = true
					result.Message = "Fedora helper was already shut off during the graceful shutdown request; retained resources unchanged"
					return result, nil
				}
			}
			if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
				return Result{}, errors.New("Fedora helper graceful shutdown timed out; domain and retained resources were not forced or removed")
			}
			return Result{}, commandFailure(downCtx, err, "Fedora helper graceful shutdown request failed")
		}
	}
	for {
		select {
		case <-downCtx.Done():
			if ctx.Err() != nil {
				return Result{}, ctx.Err()
			}
			return Result{}, errors.New("Fedora helper graceful shutdown timed out; domain and retained resources were not forced or removed")
		case <-time.After(m.PollInterval):
		}
		state, err = domstate(downCtx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
				return Result{}, errors.New("Fedora helper graceful shutdown timed out; domain and retained resources were not forced or removed")
			}
			return Result{}, err
		}
		if state == "off" {
			result.Applied = true
			result.Message = "Fedora helper shut down gracefully; networks, volumes, domain definition, and state retained"
			return result, nil
		}
	}
}

func shutdownAlreadyStopped(output []byte) bool {
	message := strings.ToLower(string(output))
	return strings.Contains(message, "not running") || strings.Contains(message, "not active") || strings.Contains(message, "already shut off")
}

func normalizeDomainState(output []byte) (string, error) {
	state := strings.Join(strings.Fields(strings.ToLower(string(output))), " ")
	switch state {
	case "running":
		return "running", nil
	case "shut off", "shutoff":
		return "off", nil
	case "in shutdown", "shutdown":
		return "stopping", nil
	case "paused", "blocked", "pmsuspended", "crashed", "dying", "no state", "nostate", "idle":
		return "", fmt.Errorf("Fedora helper domain is in unsupported %q state; refusing to guess", state)
	default:
		return "", errors.New("virsh returned an unknown Fedora helper domain state")
	}
}

func activeInterfaceCIDRs() ([]InterfaceCIDR, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var result []InterfaceCIDR
	for _, iface := range interfaces {
		addresses, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		foundIPv4 := false
		for _, value := range addresses {
			prefix, err := netip.ParsePrefix(value.String())
			if err == nil && prefix.Addr().Is4() {
				foundIPv4 = true
				result = append(result, InterfaceCIDR{Name: iface.Name, Prefix: prefix})
			}
		}
		if !foundIPv4 {
			result = append(result, InterfaceCIDR{Name: iface.Name})
		}
	}
	return result, nil
}

func secureInput(path, repoRoot string) error {
	local := filepath.Join(repoRoot, ".local") + string(os.PathSeparator)
	clean, err := filepath.Abs(path)
	if err != nil || !strings.HasPrefix(clean, local) {
		return errors.New("helper input path must remain beneath repository .local")
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(clean))
	expectedParent, expectedErr := filepath.EvalSymlinks(filepath.Join(repoRoot, ".local", "fedora-helper", "cloud-init"))
	if err != nil || expectedErr != nil || resolvedParent != expectedParent {
		return errors.New("helper cloud-init directory must not contain symlinks")
	}
	info, err := os.Lstat(clean)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return errors.New("helper cloud-init input must be a mode-0600 regular non-symlink file")
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		return errors.New("helper cloud-init input is unreadable")
	}
	var value any
	if yaml.Unmarshal(data, &value) != nil {
		return errors.New("helper cloud-init input is invalid YAML")
	}
	return nil
}

func validateCloudInputs(paths []string, h domain.Helper, c domain.ClusterDefinition) error {
	read := func(path string) []byte { data, _ := os.ReadFile(path); return data }
	user, meta, network := read(paths[0]), read(paths[1]), read(paths[2])
	var ud struct {
		Resize     bool `yaml:"resize_rootfs"`
		WriteFiles []struct {
			Path, Encoding, Content string
		} `yaml:"write_files"`
		RunCmd [][]string `yaml:"runcmd"`
	}
	if yaml.Unmarshal(user, &ud) != nil || !ud.Resize || !reflect.DeepEqual(ud.RunCmd, [][]string{{"/usr/local/sbin/okub-helper-setup"}}) {
		return errors.New("helper user-data semantics are invalid")
	}
	text := string(user)
	setupValid := false
	setupText := ""
	for _, file := range ud.WriteFiles {
		if file.Encoding != "b64" {
			return errors.New("helper service configuration must use base64 write_files")
		}
		decoded, err := base64.StdEncoding.DecodeString(file.Content)
		if err != nil {
			return errors.New("helper service configuration base64 is invalid")
		}
		if file.Path == "/usr/local/sbin/okub-helper-setup" {
			setupText = string(decoded)
			setupValid = strings.HasPrefix(setupText, "#!/bin/bash\nset -euo pipefail\n")
		}
		text += "\n" + file.Path + "\n" + string(decoded)
	}
	if !setupValid {
		return errors.New("helper setup script semantics are invalid")
	}
	packageAt := strings.Index(setupText, "dnf -y install ")
	validateAt := strings.Index(setupText, "dnsmasq --test ")
	firewallValidateAt := strings.Index(setupText, "firewall-offline-cmd --check-config")
	enableAt := strings.Index(setupText, "systemctl enable --now firewalld.service")
	if packageAt < 0 || validateAt <= packageAt || firewallValidateAt <= validateAt || enableAt <= firewallValidateAt {
		return errors.New("helper setup script is not fail-fast before service enablement")
	}
	for _, forbidden := range []string{"PRIVATE KEY", "pullSecret", "pull-secret", "password:"} {
		if strings.Contains(text, forbidden) {
			return errors.New("helper cloud-init contains forbidden secret material")
		}
	}
	if strings.Contains(text, "dhcp-boot=") {
		return errors.New("helper PXE boot must remain disabled until verified artifacts are synchronized")
	}
	for _, required := range []string{"dnsmasq", "haproxy", "nginx", "/srv/okub/http", "/srv/okub/tftp", "--test", "-c", "-t", "/etc/dnsmasq.d/okub.conf", "/etc/haproxy/haproxy.cfg", "/etc/nginx/nginx.conf", "restorecon -RF /etc/dnsmasq.d /etc/haproxy /etc/nginx", "--zone=internal --add-source=" + c.Network.MachineCIDR} {
		if !strings.Contains(text, required) {
			return errors.New("helper user-data is missing required service semantics")
		}
	}
	zone := c.Metadata.Name + "." + c.Cluster.BaseDomain
	for _, required := range []string{"interface=lan0", "host-record=api." + zone + "," + c.Network.APIVIP, "address=/apps." + zone + "/" + c.Network.IngressVIP, "bind " + c.Network.APIVIP + ":6443", "bind " + c.Network.APIVIP + ":22623", "bind " + c.Network.IngressVIP + ":80", "bind " + c.Network.IngressVIP + ":443"} {
		if !strings.Contains(text, required) {
			return errors.New("helper generated service configuration semantics are invalid")
		}
	}
	var md map[string]any
	if yaml.Unmarshal(meta, &md) != nil || md["local-hostname"] != h.VMName || md["instance-id"] == "" {
		return errors.New("helper meta-data semantics are invalid")
	}
	var nc struct {
		Version   int `yaml:"version"`
		Ethernets map[string]struct {
			Match     map[string]string `yaml:"match"`
			SetName   string            `yaml:"set-name"`
			DHCP4     bool              `yaml:"dhcp4"`
			DHCP6     bool              `yaml:"dhcp6"`
			Addresses []string          `yaml:"addresses"`
			Routes    []map[string]any  `yaml:"routes"`
			Gateway4  string            `yaml:"gateway4"`
			Gateway6  string            `yaml:"gateway6"`
		} `yaml:"ethernets"`
	}
	if yaml.Unmarshal(network, &nc) != nil {
		return errors.New("helper network-config semantics are invalid")
	}
	wanConfig, wanOK := nc.Ethernets["wan"]
	lanConfig, lanOK := nc.Ethernets["lan"]
	if nc.Version != 2 || len(nc.Ethernets) != 2 || !wanOK || !lanOK || len(wanConfig.Match) != 1 || wanConfig.Match["macaddress"] != strings.ToLower(h.WANMAC) || wanConfig.SetName != "wan0" || !wanConfig.DHCP4 || wanConfig.DHCP6 || len(wanConfig.Addresses) != 0 || len(lanConfig.Match) != 1 || lanConfig.Match["macaddress"] != strings.ToLower(h.LANMAC) || lanConfig.SetName != "lan0" || lanConfig.DHCP4 || lanConfig.DHCP6 || len(lanConfig.Routes) != 0 || lanConfig.Gateway4 != "" || lanConfig.Gateway6 != "" {
		return errors.New("helper network-config semantics are invalid")
	}
	lan, _ := netip.ParsePrefix(c.Network.MachineCIDR)
	expected := map[string]bool{fmt.Sprintf("%s/%d", lan.Addr().Next(), lan.Bits()): true, c.Network.APIVIP + "/32": true, c.Network.IngressVIP + "/32": true}
	if len(lanConfig.Addresses) != len(expected) {
		return errors.New("helper LAN addresses are incomplete")
	}
	for _, a := range lanConfig.Addresses {
		if !expected[a] {
			return errors.New("helper LAN addresses are incomplete")
		}
		delete(expected, a)
	}
	if len(expected) != 0 {
		return errors.New("helper LAN addresses are incomplete")
	}
	return nil
}

func bridgeNames(vmName string) (string, string) {
	digest := fmt.Sprintf("%x", md5.Sum([]byte(vmName)))[:8]
	return "okub-" + digest + "w", "okub-" + digest + "l"
}

func validateHostNetworks(interfaces []InterfaceCIDR, present map[string]bool, wanBridge, lanBridge, wanCIDR, lanCIDR string) ([]string, error) {
	wan, wanErr := netip.ParsePrefix(wanCIDR)
	lan, lanErr := netip.ParsePrefix(lanCIDR)
	if wanErr != nil || lanErr != nil {
		return nil, errors.New("invalid managed network CIDR")
	}
	wan, lan = wan.Masked(), lan.Masked()
	wantWAN := netip.PrefixFrom(wan.Addr().Next(), wan.Bits())
	wantLAN := netip.PrefixFrom(lastIPv4Address(lan).Prev(), lan.Bits())
	var excluded []netip.Prefix
	for _, iface := range interfaces {
		prefix := iface.Prefix
		if (iface.Name == wanBridge || iface.Name == lanBridge) && (!prefix.IsValid() || !prefix.Addr().Is4()) {
			return nil, fmt.Errorf("owned bridge %s has no IPv4 address", iface.Name)
		}
		if !prefix.IsValid() || !prefix.Addr().Is4() {
			continue
		}
		address := ""
		if iface.Name == wanBridge {
			address = "libvirt_network.wan"
		} else if iface.Name == lanBridge {
			address = "libvirt_network.lan"
		}
		if address != "" && present[address] {
			want := wantWAN
			if iface.Name == lanBridge {
				want = wantLAN
			}
			if prefix != want {
				return nil, fmt.Errorf("owned bridge %s has unexpected CIDR %s", iface.Name, prefix)
			}
			continue
		}
		if prefix.Masked().Overlaps(wan) || prefix.Masked().Overlaps(lan) {
			return nil, fmt.Errorf("managed Fedora helper network overlaps active host interface %s (%s)", iface.Name, prefix)
		}
		excluded = append(excluded, prefix)
	}
	// A state-owned libvirt network can be stopped after a host reboot, so its
	// bridge may be absent. Any bridge that does exist must still match exactly.
	return canonicalPrefixes(excluded), nil
}

func lastIPv4Address(prefix netip.Prefix) netip.Addr {
	b := prefix.Masked().Addr().As4()
	value := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	value |= uint32((uint64(1) << (32 - prefix.Bits())) - 1)
	return netip.AddrFrom4([4]byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)})
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type domainIdentity struct {
	UUID string
}

func stateDomainIdentity(data []byte, configuredName string) (domainIdentity, error) {
	type module struct {
		Resources []struct {
			Address string         `json:"address"`
			Values  map[string]any `json:"values"`
		} `json:"resources"`
		ChildModules []module `json:"child_modules"`
	}
	var state struct {
		Values struct {
			RootModule module `json:"root_module"`
		} `json:"values"`
	}
	if json.Unmarshal(data, &state) != nil {
		return domainIdentity{}, errors.New("current OpenTofu state JSON is invalid")
	}
	var matches []map[string]any
	var walk func(module)
	walk = func(current module) {
		for _, resource := range current.Resources {
			if resource.Address == "libvirt_domain.helper" {
				matches = append(matches, resource.Values)
			}
		}
		for _, child := range current.ChildModules {
			walk(child)
		}
	}
	walk(state.Values.RootModule)
	if len(matches) != 1 {
		return domainIdentity{}, errors.New("current OpenTofu state must contain exactly one owned Fedora helper domain identity")
	}
	name, nameOK := matches[0]["name"].(string)
	uuid, uuidOK := matches[0]["id"].(string)
	uuid = strings.ToLower(uuid)
	if !nameOK || name != configuredName || !uuidOK || !uuidPattern.MatchString(uuid) {
		return domainIdentity{}, errors.New("owned Fedora helper domain state identity is missing, malformed, or does not match configuration")
	}
	return domainIdentity{UUID: uuid}, nil
}

func canonicalPrefixes(prefixes []netip.Prefix) []string {
	seen := map[string]bool{}
	for _, prefix := range prefixes {
		if prefix.Addr().Is4() {
			seen[prefix.Masked().String()] = true
		}
	}
	result := make([]string, 0, len(seen))
	for prefix := range seen {
		result = append(result, prefix)
	}
	sort.Strings(result)
	return result
}

func (m Manager) finish(runtime, operation string, p domain.Helper, log []byte, returned error) error {
	for _, v := range []string{p.LibvirtURI} {
		if v != "" {
			log = []byte(strings.ReplaceAll(string(log), v, "[REDACTED]"))
		}
	}
	if err := writePrivateLog(filepath.Join(runtime, operation+".log"), log); err != nil {
		return err
	}
	return returned
}

func acquireOperationLock(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open Fedora helper operation lock: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0o777 != 0o600 || stat.Nlink != 1 {
		file.Close()
		return nil, errors.New("Fedora helper operation lock is not a secure mode-0600 regular file")
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, errors.New("another Fedora helper lifecycle operation is in progress")
		}
		return nil, fmt.Errorf("lock Fedora helper lifecycle operation: %w", err)
	}
	return file, nil
}

func reservePlanPath(runtime, operation string) (string, error) {
	file, err := os.CreateTemp(runtime, "."+operation+"-*.tfplan")
	if err != nil {
		return "", fmt.Errorf("reserve unique plan path: %w", err)
	}
	path := file.Name()
	if err := file.Chmod(0o600); err == nil {
		err = file.Close()
	} else {
		_ = file.Close()
	}
	if err == nil {
		err = os.Remove(path)
	}
	if err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("reserve unique plan path: %w", err)
	}
	return path, nil
}

func securePlan(path string) error {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return errors.New("tofu did not create a regular saved plan")
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		return errors.New("tofu did not create a regular saved plan")
	}
	// OpenTofu commonly creates plans as 0644. The parent is private, so tighten
	// the verified inode before it is inspected or applied.
	if err := unix.Fchmod(fd, 0o600); err != nil || unix.Fstat(fd, &stat) != nil || stat.Mode&0o777 != 0o600 {
		return errors.New("tofu did not create a regular saved plan")
	}
	return nil
}

func secureOptionalRuntimeFile(path string) error {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("runtime file %s is unsafe", filepath.Base(path))
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0o777 != 0o600 || stat.Nlink != 1 {
		return fmt.Errorf("runtime file %s must be a mode-0600 regular non-symlink file", filepath.Base(path))
	}
	return nil
}

func secureManagedRuntimeFile(path string) error {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("runtime file %s is unsafe", filepath.Base(path))
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		return fmt.Errorf("runtime file %s must be a regular non-symlink file", filepath.Base(path))
	}
	if err := unix.Fchmod(fd, 0o600); err != nil || unix.Fstat(fd, &stat) != nil || stat.Mode&0o777 != 0o600 {
		return fmt.Errorf("runtime file %s could not be secured", filepath.Base(path))
	}
	return nil
}

func writePrivateLog(path string, data []byte) error {
	if err := secureOptionalRuntimeFile(path); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".operation-log-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	return err
}

func commandFailure(ctx context.Context, err error, message string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.New(message)
}

var ownedAddresses = map[string]bool{
	"libvirt_network.wan":         true,
	"libvirt_network.lan":         true,
	"libvirt_volume.base_image":   true,
	"libvirt_volume.root":         true,
	"libvirt_cloudinit_disk.seed": true,
	"libvirt_domain.helper":       true,
}

func ownedState(data []byte) (map[string]bool, error) {
	present := map[string]bool{}
	for _, address := range strings.Fields(string(data)) {
		if !ownedAddresses[address] {
			return nil, fmt.Errorf("Fedora helper state contains unexpected address %q", address)
		}
		present[address] = true
	}
	return present, nil
}

func missingOwnedAddresses(present map[string]bool) []string {
	missing := make([]string, 0, len(ownedAddresses)-len(present))
	for address := range ownedAddresses {
		if !present[address] {
			missing = append(missing, address)
		}
	}
	sort.Strings(missing)
	return missing
}

func validatePlan(data []byte, present map[string]bool) (bool, error) {
	var plan struct {
		ResourceChanges []struct {
			Address string `json:"address"`
			Change  struct {
				Actions []string       `json:"actions"`
				Before  map[string]any `json:"before"`
				After   map[string]any `json:"after"`
			} `json:"change"`
		} `json:"resource_changes"`
	}
	if json.Unmarshal(data, &plan) != nil {
		return false, errors.New("tofu plan JSON is invalid")
	}
	meaningful := plan.ResourceChanges[:0]
	for _, r := range plan.ResourceChanges {
		if !ownedAddresses[r.Address] {
			return false, fmt.Errorf("unsafe Fedora helper plan contains unexpected address %q", r.Address)
		}
		if len(r.Change.Actions) == 1 && r.Change.Actions[0] == "no-op" {
			if !present[r.Address] {
				return false, fmt.Errorf("unsafe Fedora helper plan treats missing resource %q as present", r.Address)
			}
			continue
		}
		meaningful = append(meaningful, r)
	}
	missing := missingOwnedAddresses(present)
	if len(missing) > 0 {
		if len(meaningful) != len(missing) {
			return false, errors.New("unsafe Fedora helper recovery plan does not create exactly the missing owned resources")
		}
		created := map[string]bool{}
		for _, r := range meaningful {
			if present[r.Address] || !reflect.DeepEqual(r.Change.Actions, []string{"create"}) || r.Change.Before != nil {
				return false, errors.New("unsafe Fedora helper recovery plan contains an unexpected action")
			}
			if r.Address == "libvirt_domain.helper" && r.Change.After["running"] != true {
				return false, errors.New("unsafe Fedora helper recovery plan does not create a running domain")
			}
			created[r.Address] = true
		}
		for _, address := range missing {
			if !created[address] {
				return false, errors.New("unsafe Fedora helper recovery plan does not create every missing owned resource")
			}
		}
		return true, nil
	}
	if len(meaningful) == 0 {
		return false, nil
	}
	if len(meaningful) != 1 || meaningful[0].Address != "libvirt_domain.helper" || !reflect.DeepEqual(meaningful[0].Change.Actions, []string{"update"}) {
		return false, errors.New("unsafe Fedora helper plan changes resources other than domain power state")
	}
	b, a := meaningful[0].Change.Before, meaningful[0].Change.After
	if b["running"] != false || a["running"] != true {
		return false, errors.New("unsafe Fedora helper plan has an unexpected running transition")
	}
	delete(b, "running")
	delete(a, "running")
	if !reflect.DeepEqual(b, a) {
		return false, errors.New("unsafe Fedora helper plan changes values other than running")
	}
	return true, nil
}
