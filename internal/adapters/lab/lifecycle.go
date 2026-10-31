// Package lab safely orchestrates the disposable libvirt validation harness.
package lab

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
	"golang.org/x/sys/unix"
)

type Runner interface {
	Run(context.Context, string, []string, ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, directory string, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = directory
	cmd.Env = cleanEnvironment(os.Environ(), env)
	return cmd.CombinedOutput()
}

func cleanEnvironment(base, overrides []string) []string {
	result := make([]string, 0, len(base)+len(overrides))
	for _, value := range base {
		name, _, _ := strings.Cut(value, "=")
		if !strings.HasPrefix(name, "TF_") && !strings.HasPrefix(name, "TOFU_") {
			result = append(result, value)
		}
	}
	return append(result, overrides...)
}

type Manager struct {
	Runner   Runner
	LookPath func(string) (string, error)
	OpenKVM  func() error
	Source   ports.ArtifactSetSource
}

type Request struct {
	RunID, Manifest, StoragePool, Firmware string
	MemoryMiB, VCPU, DiskGiB               int
}

type Result struct {
	Operation       string `json:"operation"`
	RunID           string `json:"runID"`
	Planned         bool   `json:"planned"`
	Applied         bool   `json:"applied"`
	Changed         bool   `json:"changed"`
	Message         string `json:"message"`
	DiagnosticsPath string `json:"diagnosticsPath,omitempty"`
}

var runIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

var ownedAddresses = map[string]bool{
	"libvirt_network.lab": true, "libvirt_volume.root": true, "libvirt_domain.sno": true,
}

func (m Manager) Run(ctx context.Context, repo string, definition domain.ClusterDefinition, operation string, request Request, apply bool) (result Result, returned error) {
	result = Result{Operation: operation, RunID: request.RunID}
	if operation != "up" && operation != "down" {
		return result, errors.New("invalid lab operation")
	}
	if !runIDPattern.MatchString(request.RunID) {
		return result, errors.New("--run must be a lowercase RFC 4122 version-4 UUID")
	}
	if definition.Platform.Type != "none" || definition.Cluster.Topology != "sno" || len(definition.Hosts) != 1 || len(definition.Hosts[0].Interfaces) == 0 {
		return result, errors.New("libvirt lab requires a normal platform:none SNO definition with one configured host interface")
	}
	m.defaults()
	if operation != "down" {
		if err := m.OpenKVM(); err != nil {
			return result, errors.New("KVM is unavailable or inaccessible; run the libvirt lab on a KVM-capable host")
		}
	}
	tofu, err := m.LookPath("tofu")
	if err != nil {
		return result, errors.New("tofu is unavailable")
	}
	virsh, err := m.LookPath("virsh")
	if err != nil {
		return result, errors.New("virsh is unavailable")
	}
	module := filepath.Join(repo, "lab", "terraform", "libvirt")
	runtime := filepath.Join(repo, ".local", "lab", request.RunID)
	if err := os.MkdirAll(runtime, 0o700); err != nil {
		return result, err
	}
	if err := os.Chmod(runtime, 0o700); err != nil {
		return result, err
	}
	logPath := filepath.Join(runtime, operation+".log")
	var log []byte
	defer func() {
		if writeErr := writePrivate(logPath, redact(log, definition)); returned == nil && writeErr != nil {
			returned = writeErr
		}
	}()
	env := []string{"TF_IN_AUTOMATION=1", "TF_DATA_DIR=" + filepath.Join(runtime, "tofu")}
	run := func(args ...string) ([]byte, error) {
		out, runErr := m.Runner.Run(ctx, module, env, args...)
		log = append(log, out...)
		return out, runErr
	}
	if out, err := run(virsh, "-c", "qemu:///system", "uri"); err != nil || strings.TrimSpace(string(out)) != "qemu:///system" {
		return result, errors.New("local qemu:///system libvirt is unavailable")
	}
	if _, err := run(tofu, "init", "-input=false", "-backend-config=path="+filepath.Join(runtime, "terraform.tfstate")); err != nil {
		return result, errors.New("tofu init failed")
	}
	listed, listErr := run(tofu, "state", "list")
	if listErr != nil && !strings.Contains(string(listed), "No state file was found") {
		return result, errors.New("cannot inspect lab state")
	}
	present, err := ownedState(listed)
	if err != nil {
		return result, err
	}
	if operation == "down" {
		if len(present) == 0 {
			result.Planned, result.Message = true, "already down"
			return result, nil
		}
		if len(present) != len(ownedAddresses) {
			return result, errors.New("lab state is partial; refusing destroy")
		}
		if err := verifyRunVariables(filepath.Join(runtime, "run.tfvars.json"), request.RunID); err != nil {
			return result, err
		}
		if err := inspectIdentity(ctx, m.Runner, module, env, tofu, virsh, request.RunID); err != nil {
			return result, err
		}
		return m.destroy(ctx, module, runtime, env, tofu, request, apply, run, result)
	}
	if request.StoragePool == "" || request.Firmware == "" || request.Manifest == "" {
		return result, errors.New("up requires --manifest, --pool, and --firmware")
	}
	if !filepath.IsAbs(request.Firmware) {
		return result, errors.New("--firmware must be an absolute path")
	}
	if info, statErr := os.Stat(request.Firmware); statErr != nil || !info.Mode().IsRegular() {
		return result, errors.New("--firmware must identify an existing regular file")
	}
	set, err := m.Source.LoadAndVerify(request.Manifest)
	if err != nil {
		return result, fmt.Errorf("verify published artifacts: %w", err)
	}
	iso := ""
	if definition.Boot.Method == "iso" {
		for _, artifact := range set.Artifacts {
			if strings.HasSuffix(artifact.Path, ".iso") {
				iso = artifact.FilePath
			}
		}
		if iso == "" {
			return result, errors.New("ISO boot requires a published ISO artifact set")
		}
	} else if definition.Boot.Method == "pxe" {
		if len(set.Artifacts) < 4 {
			return result, errors.New("PXE boot requires a published PXE artifact set")
		}
	} else {
		return result, errors.New("lab boot method must be iso or pxe")
	}
	if len(present) != 0 && len(present) != len(ownedAddresses) {
		return result, errors.New("lab state is partial; refusing to guess ownership recovery")
	}
	variableFile := filepath.Join(runtime, "run.tfvars.json")
	bridgeCIDR, err := labBridgeCIDR(definition.Network.MachineCIDR)
	if err != nil {
		return result, err
	}
	variableValues := map[string]any{
		"run_id": request.RunID, "storage_pool": request.StoragePool, "bridge_cidr": bridgeCIDR,
		"host_mac": definition.Hosts[0].Interfaces[0].MACAddress, "cluster_domain": definition.Metadata.Name + "." + definition.Cluster.BaseDomain,
		"api_vip": definition.Network.APIVIP, "ingress_vip": definition.Network.IngressVIP, "boot_method": definition.Boot.Method,
		"iso_path": iso, "firmware": request.Firmware, "memory_mib": request.MemoryMiB, "vcpu": request.VCPU, "disk_gib": request.DiskGiB,
	}
	if err := persistRunVariables(variableFile, variableValues); err != nil {
		return result, err
	}
	plan := filepath.Join(runtime, "up.tfplan")
	args := []string{tofu, "plan", "-input=false", "-lock-timeout=30s", "-out=" + plan, "-var-file=" + variableFile}
	if _, err := run(args...); err != nil {
		return result, errors.New("tofu plan failed")
	}
	shown, err := run(tofu, "show", "-json", plan)
	if err != nil {
		return result, errors.New("cannot inspect lab plan")
	}
	changed, err := validatePlan(shown, operation)
	if err != nil {
		return result, err
	}
	result.Planned, result.Changed = true, changed
	if !changed {
		result.Message = "already up"
		return result, nil
	}
	if !apply {
		result.Message = "safe plan created; pass --apply to authorize host mutation"
		return result, nil
	}
	if _, err := run(tofu, "apply", "-input=false", "-auto-approve", plan); err != nil {
		return result, errors.New("tofu apply failed")
	}
	result.Applied, result.Message = true, "owned SNO lab resources created"
	if definition.Boot.Method == "pxe" {
		result.Message += "; production PXE artifact serving must remain active"
	}
	return result, nil
}

func (m *Manager) defaults() {
	if m.Runner == nil {
		m.Runner = ExecRunner{}
	}
	if m.LookPath == nil {
		m.LookPath = exec.LookPath
	}
	if m.OpenKVM == nil {
		m.OpenKVM = func() error {
			f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
			if err != nil {
				return err
			}
			defer f.Close()
			version, err := unix.IoctlGetInt(int(f.Fd()), 0xAE00) // KVM_GET_API_VERSION
			if err != nil {
				return err
			}
			if version != 12 {
				return fmt.Errorf("unsupported KVM API version %d", version)
			}
			return nil
		}
	}
}

func labBridgeCIDR(machineCIDR string) (string, error) {
	prefix, err := netip.ParsePrefix(machineCIDR)
	if err != nil || !prefix.Addr().Is4() || prefix.Bits() > 30 {
		return "", errors.New("lab machine CIDR must be an IPv4 network with a usable bridge address")
	}
	bytes := prefix.Masked().Addr().As4()
	base := binary.BigEndian.Uint32(bytes[:])
	broadcast := base | ^uint32(0)>>(prefix.Bits())
	bridge := broadcast - 1
	var address [4]byte
	binary.BigEndian.PutUint32(address[:], bridge)
	return netip.PrefixFrom(netip.AddrFrom4(address), prefix.Bits()).String(), nil
}

func (m Manager) destroy(_ context.Context, _ string, runtime string, _ []string, tofu string, _ Request, apply bool, run func(...string) ([]byte, error), result Result) (Result, error) {
	plan := filepath.Join(runtime, "down.tfplan")
	if _, err := run(tofu, "plan", "-destroy", "-input=false", "-lock-timeout=30s", "-out="+plan, "-var-file="+filepath.Join(runtime, "run.tfvars.json")); err != nil {
		return result, errors.New("tofu destroy plan failed")
	}
	shown, err := run(tofu, "show", "-json", plan)
	if err != nil {
		return result, errors.New("cannot inspect destroy plan")
	}
	changed, err := validatePlan(shown, "down")
	if err != nil {
		return result, err
	}
	result.Planned, result.Changed = true, changed
	if !apply {
		result.Message = "safe exact-owner destroy plan created; pass --apply to authorize host mutation"
		return result, nil
	}
	if _, err := run(tofu, "apply", "-input=false", "-auto-approve", plan); err != nil {
		return result, errors.New("tofu destroy apply failed")
	}
	result.Applied, result.Message = true, "exact owned lab resources destroyed"
	return result, nil
}

func persistRunVariables(path string, values map[string]any) error {
	data, err := json.Marshal(values)
	if err != nil {
		return err
	}
	if existing, readErr := os.ReadFile(path); readErr == nil {
		var current map[string]any
		if json.Unmarshal(existing, &current) != nil || !reflect.DeepEqual(current, decodeJSONMap(data)) {
			return errors.New("existing run identifier belongs to different lab inputs")
		}
		return nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return errors.New("cannot inspect saved lab inputs")
	}
	return writePrivate(path, append(data, '\n'))
}

func decodeJSONMap(data []byte) map[string]any {
	var value map[string]any
	_ = json.Unmarshal(data, &value)
	return value
}

func verifyRunVariables(path, runID string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return errors.New("saved lab inputs are unavailable; refusing destroy")
	}
	var values map[string]any
	if json.Unmarshal(data, &values) != nil || values["run_id"] != runID {
		return errors.New("saved lab ownership identifier does not match --run")
	}
	return nil
}

type domainXML struct {
	Description string `xml:"description"`
	Devices     struct {
		Disks      []domainDisk      `xml:"disk"`
		Interfaces []domainInterface `xml:"interface"`
	} `xml:"devices"`
}

type domainDisk struct {
	Device string `xml:"device,attr"`
	Source struct {
		File string `xml:"file,attr"`
	} `xml:"source"`
	ReadOnly *struct{} `xml:"readonly"`
}

type domainInterface struct {
	Source struct {
		Network string `xml:"network,attr"`
	} `xml:"source"`
}

func inspectIdentity(ctx context.Context, runner Runner, module string, env []string, tofu, virsh, runID string) error {
	data, err := runner.Run(ctx, module, env, tofu, "show", "-json")
	if err != nil {
		return errors.New("cannot inspect lab state identity")
	}
	var state struct {
		Values struct {
			Outputs map[string]struct {
				Value any `json:"value"`
			} `json:"outputs"`
			Root struct {
				Resources []struct {
					Address string         `json:"address"`
					Values  map[string]any `json:"values"`
				} `json:"resources"`
			} `json:"root_module"`
		} `json:"values"`
	}
	if json.Unmarshal(data, &state) != nil || state.Values.Outputs["run_id"].Value != runID {
		return errors.New("lab state ownership identifier does not match --run")
	}
	tag := "okub-lab-" + strings.ReplaceAll(runID, "-", "")[:12]
	expected := map[string]string{
		"libvirt_domain.sno":  tag + "-sno",
		"libvirt_network.lab": tag + "-net",
		"libvirt_volume.root": tag + "-root.qcow2",
	}
	identities := map[string]map[string]string{}
	for _, r := range state.Values.Root.Resources {
		if expectedName, owned := expected[r.Address]; owned {
			name, _ := r.Values["name"].(string)
			id, _ := r.Values["id"].(string)
			if name != expectedName || id == "" {
				return fmt.Errorf("%s identity does not match run ownership", r.Address)
			}
			identities[r.Address] = map[string]string{"name": name, "id": id}
			if pool, ok := r.Values["pool"].(string); ok {
				identities[r.Address]["pool"] = pool
			}
		}
	}
	if len(identities) != len(expected) {
		return errors.New("lab resource identity is incomplete in state")
	}
	domain, network, volume := identities["libvirt_domain.sno"], identities["libvirt_network.lab"], identities["libvirt_volume.root"]
	actual, err := runner.Run(ctx, module, env, virsh, "-c", "qemu:///system", "domuuid", domain["name"])
	if err != nil || !strings.EqualFold(strings.TrimSpace(string(actual)), domain["id"]) {
		return errors.New("live lab domain does not match inspected state identity")
	}
	actual, err = runner.Run(ctx, module, env, virsh, "-c", "qemu:///system", "net-uuid", network["name"])
	if err != nil || !strings.EqualFold(strings.TrimSpace(string(actual)), network["id"]) {
		return errors.New("live lab network does not match inspected state identity")
	}
	if volume["pool"] == "" {
		return errors.New("lab volume pool identity is missing from state")
	}
	actual, err = runner.Run(ctx, module, env, virsh, "-c", "qemu:///system", "vol-path", "--pool", volume["pool"], volume["name"])
	if err != nil || filepath.Clean(strings.TrimSpace(string(actual))) != filepath.Clean(volume["id"]) {
		return errors.New("live lab volume does not match inspected state identity")
	}
	xmlData, err := runner.Run(ctx, module, env, virsh, "-c", "qemu:///system", "dumpxml", domain["name"])
	var live domainXML
	if err != nil || xml.Unmarshal(xmlData, &live) != nil || live.Description != "Okub validation lab owner="+runID || !domainUses(live.Devices.Disks, live.Devices.Interfaces, volume["id"], network["name"]) {
		return errors.New("live lab domain is not attached exclusively to the owned network and volume")
	}
	names, err := runner.Run(ctx, module, env, virsh, "-c", "qemu:///system", "list", "--all", "--name")
	if err != nil {
		return errors.New("cannot inspect other libvirt domain attachments")
	}
	for _, name := range strings.Fields(string(names)) {
		if name == domain["name"] {
			continue
		}
		otherXML, err := runner.Run(ctx, module, env, virsh, "-c", "qemu:///system", "dumpxml", name)
		var other domainXML
		if err != nil || xml.Unmarshal(otherXML, &other) != nil {
			return errors.New("cannot inspect other libvirt domain attachments")
		}
		if domainConsumes(other, volume["id"], network["name"]) {
			return fmt.Errorf("owned lab network or volume is attached to foreign domain %q", name)
		}
	}
	return nil
}

func domainUses(disks []domainDisk, interfaces []domainInterface, volume, network string) bool {
	if len(interfaces) != 1 || interfaces[0].Source.Network != network {
		return false
	}
	root := 0
	for _, disk := range disks {
		if filepath.Clean(disk.Source.File) == filepath.Clean(volume) {
			root++
			continue
		}
		if disk.Device != "cdrom" || disk.ReadOnly == nil {
			return false
		}
	}
	return root == 1
}

func domainConsumes(domain domainXML, volume, network string) bool {
	for _, disk := range domain.Devices.Disks {
		if filepath.Clean(disk.Source.File) == filepath.Clean(volume) {
			return true
		}
	}
	for _, nic := range domain.Devices.Interfaces {
		if nic.Source.Network == network {
			return true
		}
	}
	return false
}

func ownedState(data []byte) (map[string]bool, error) {
	present := map[string]bool{}
	for _, address := range strings.Fields(string(data)) {
		if !ownedAddresses[address] {
			return nil, fmt.Errorf("lab state contains unexpected address %q", address)
		}
		present[address] = true
	}
	return present, nil
}

func validatePlan(data []byte, operation string) (bool, error) {
	var plan struct {
		Changes []struct {
			Address string `json:"address"`
			Change  struct {
				Actions []string `json:"actions"`
			} `json:"change"`
		} `json:"resource_changes"`
	}
	if json.Unmarshal(data, &plan) != nil {
		return false, errors.New("tofu plan JSON is invalid")
	}
	seen, changed := map[string]bool{}, false
	for _, r := range plan.Changes {
		if !ownedAddresses[r.Address] {
			return false, fmt.Errorf("unsafe lab plan contains unexpected address %q", r.Address)
		}
		seen[r.Address] = true
		if reflect.DeepEqual(r.Change.Actions, []string{"no-op"}) {
			continue
		}
		want := []string{"create"}
		if operation == "down" {
			want = []string{"delete"}
		}
		if !reflect.DeepEqual(r.Change.Actions, want) {
			return false, fmt.Errorf("unsafe lab plan has %v for %s", r.Change.Actions, r.Address)
		}
		changed = true
	}
	if len(seen) != len(ownedAddresses) {
		keys := make([]string, 0, len(seen))
		for k := range seen {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return false, fmt.Errorf("lab plan resource set is incomplete: %v", keys)
	}
	return changed, nil
}

func redact(data []byte, definition domain.ClusterDefinition) []byte {
	text := string(data)
	for _, secret := range []string{definition.Credentials.PullSecretFile, definition.Credentials.SSHPublicKeyFile} {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	return []byte(text)
}

func writePrivate(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".lab-log-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(data)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	return err
}

func (m Manager) Diagnostics(ctx context.Context, repo, runID string) (Result, error) {
	result := Result{Operation: "test", RunID: runID}
	if !runIDPattern.MatchString(runID) {
		return result, errors.New("--run must be a lowercase RFC 4122 version-4 UUID")
	}
	m.defaults()
	if err := m.OpenKVM(); err != nil {
		return result, errors.New("KVM is unavailable or inaccessible; privileged lab E2E is pending on a KVM-capable host")
	}
	tofu, e1 := m.LookPath("tofu")
	virsh, e2 := m.LookPath("virsh")
	if e1 != nil || e2 != nil {
		return result, errors.New("tofu and virsh are required")
	}
	module, runtime := filepath.Join(repo, "lab", "terraform", "libvirt"), filepath.Join(repo, ".local", "lab", runID)
	env := []string{"TF_IN_AUTOMATION=1", "TF_DATA_DIR=" + filepath.Join(runtime, "tofu")}
	if err := inspectIdentity(ctx, m.Runner, module, env, tofu, virsh, runID); err != nil {
		return result, err
	}
	show, _ := m.Runner.Run(ctx, module, env, tofu, "output", "-json")
	var outputs map[string]struct {
		Value any `json:"value"`
	}
	if json.Unmarshal(show, &outputs) != nil {
		return result, errors.New("cannot inspect lab outputs")
	}
	name, _ := outputs["domain_name"].Value.(string)
	state, err := m.Runner.Run(ctx, module, env, virsh, "-c", "qemu:///system", "domstate", name)
	if err != nil {
		return result, errors.New("cannot inspect lab domain state")
	}
	serialPath := filepath.Join(runtime, "serial.log")
	if err := writePrivate(serialPath, []byte("serial output is not persisted because it may contain credentials\n")); err != nil {
		return result, err
	}
	diagnostic := struct {
		RunID, Domain, State, SerialLog string
		CapturedAt                      time.Time
	}{runID, name, strings.TrimSpace(string(state)), serialPath, time.Now().UTC()}
	data, _ := json.MarshalIndent(diagnostic, "", "  ")
	data = append(data, '\n')
	path := filepath.Join(runtime, "diagnostics.json")
	if err := writePrivate(path, data); err != nil {
		return result, err
	}
	result.DiagnosticsPath = path
	if !strings.EqualFold(diagnostic.State, "running") {
		return result, errors.New("lab domain is not running; redacted diagnostics captured")
	}
	result.Message = "lab domain is running; installation checkpoint must still be observed by the normal installer monitor"
	return result, nil
}
