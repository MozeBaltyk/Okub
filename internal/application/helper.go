package application

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/mozebaltyk/Okub/internal/domain"
	"go.yaml.in/yaml/v3"
)

type HelperRenderResult struct {
	Directory     string `json:"directory"`
	UserData      string `json:"userData"`
	MetaData      string `json:"metaData"`
	NetworkConfig string `json:"networkConfig"`
}

// RenderHelper writes deterministic, secret-free NoCloud inputs beneath .local.
func RenderHelper(configPath, repoRoot string, validator ConfigurationValidator) (HelperRenderResult, domain.ValidationReport, error) {
	validated, err := validator.Validate(configPath)
	if err != nil || !validated.Report.Valid {
		return HelperRenderResult{}, validated.Report, err
	}
	c := validated.Definition
	if c.Helper == nil {
		return HelperRenderResult{}, validated.Report, errors.New("helper configuration is required")
	}
	dir := filepath.Join(repoRoot, ".local", "fedora-helper", "cloud-init")
	if err := makePrivatePath(repoRoot, ".local", "fedora-helper", "cloud-init"); err != nil {
		return HelperRenderResult{}, validated.Report, fmt.Errorf("create helper cloud-init directory: %w", err)
	}
	user, meta, network, err := helperCloudInit(c)
	if err != nil {
		return HelperRenderResult{}, validated.Report, err
	}
	paths := []string{filepath.Join(dir, "user-data.yaml"), filepath.Join(dir, "meta-data.yaml"), filepath.Join(dir, "network-config.yaml")}
	for i, data := range [][]byte{user, meta, network} {
		if err := writePrivateAtomic(paths[i], data); err != nil {
			return HelperRenderResult{}, validated.Report, err
		}
	}
	return HelperRenderResult{dir, paths[0], paths[1], paths[2]}, validated.Report, nil
}

func makePrivatePath(root string, parts ...string) error {
	current := root
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err = os.Mkdir(current, 0o700); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
			return errors.New("helper runtime path contains a symlink, non-directory, or nonprivate directory")
		}
	}
	return nil
}

func helperCloudInit(c domain.ClusterDefinition) ([]byte, []byte, []byte, error) {
	h := c.Helper
	lan := netip.MustParsePrefix(c.Network.MachineCIDR)
	helperIP := lan.Addr().Next()
	zone := c.Metadata.Name + "." + c.Cluster.BaseDomain
	dns := []string{"interface=lan0", "bind-dynamic", "domain=" + zone, "local=/" + zone + "/", "dhcp-authoritative", "dhcp-option=option:router," + helperIP.String(), "dhcp-option=option:dns-server," + helperIP.String()}
	start, end := dhcpRange(lan, c)
	dns = append(dns, "dhcp-range="+start.String()+","+end.String()+",12h", "host-record=api."+zone+","+c.Network.APIVIP, "host-record=api-int."+zone+","+c.Network.APIVIP, "address=/apps."+zone+"/"+c.Network.IngressVIP)
	for _, host := range c.Hosts {
		p, _ := netip.ParsePrefix(host.Address)
		dns = append(dns, "host-record="+host.Name+"."+zone+","+p.Addr().String(), "dhcp-host="+strings.ToLower(host.Interfaces[0].MACAddress)+","+p.Addr().String()+","+host.Name+",infinite")
	}
	if c.Boot.Method == "pxe" {
		dns = append(dns, "enable-tftp", "tftp-root=/srv/okub/tftp")
	}
	dns = append(dns, "")
	backends := c.Hosts
	workers := []domain.Host{}
	masters := []domain.Host{}
	for _, host := range c.Hosts {
		if host.Role == "worker" {
			workers = append(workers, host)
		} else {
			masters = append(masters, host)
		}
	}
	if len(workers) > 0 {
		backends = workers
	} else {
		backends = masters
	}
	haproxy := []string{"global", "  log 127.0.0.1 local2", "defaults", "  log global", "  mode tcp", "  timeout connect 10s", "  timeout client 1m", "  timeout server 1m"}
	addProxy := func(name, bind string, port int, hosts []domain.Host) {
		haproxy = append(haproxy, "frontend "+name, "  bind "+bind+fmt.Sprintf(":%d", port), "  default_backend "+name, "backend "+name)
		for _, host := range hosts {
			p, _ := netip.ParsePrefix(host.Address)
			haproxy = append(haproxy, "  server "+host.Name+" "+p.Addr().String()+fmt.Sprintf(":%d check", port))
		}
	}
	addProxy("api", c.Network.APIVIP, 6443, masters)
	addProxy("mcs", c.Network.APIVIP, 22623, masters)
	addProxy("ingress-http", c.Network.IngressVIP, 80, backends)
	addProxy("ingress-https", c.Network.IngressVIP, 443, backends)
	haproxy = append(haproxy, "")
	nginx := fmt.Sprintf("user nginx;\nworker_processes auto;\nerror_log /var/log/nginx/error.log notice;\npid /run/nginx.pid;\nevents { worker_connections 1024; }\nhttp {\n  include /etc/nginx/mime.types;\n  default_type application/octet-stream;\n  sendfile on;\n  server {\n    listen %s:8080;\n    server_name _;\n    root /srv/okub/http;\n    autoindex off;\n  }\n}\n", helperIP)
	write := func(path, content string) map[string]any {
		return map[string]any{"path": path, "owner": "root:root", "permissions": "0644", "encoding": "b64", "content": base64.StdEncoding.EncodeToString([]byte(content))}
	}
	user := map[string]any{"hostname": h.VMName, "manage_etc_hosts": true, "ssh_pwauth": false, "disable_root": true, "growpart": map[string]any{"mode": "auto", "devices": []string{"/"}}, "resize_rootfs": true, "write_files": []map[string]any{write("/etc/sysctl.d/90-okub-router.conf", "net.ipv4.ip_forward = 1\n"), write("/etc/dnsmasq.d/okub.conf", strings.Join(dns, "\n")), write("/etc/haproxy/haproxy.cfg", strings.Join(haproxy, "\n")), write("/etc/nginx/nginx.conf", nginx)}}
	setupScript := strings.Replace(helperSetupScript, "@LAN_CIDR@", c.Network.MachineCIDR, 1)
	user["write_files"] = append(user["write_files"].([]map[string]any), map[string]any{"path": "/usr/local/sbin/okub-helper-setup", "owner": "root:root", "permissions": "0700", "encoding": "b64", "content": base64.StdEncoding.EncodeToString([]byte(setupScript))})
	user["runcmd"] = [][]string{{"/usr/local/sbin/okub-helper-setup"}}
	net := map[string]any{"version": 2, "ethernets": map[string]any{"wan": map[string]any{"match": map[string]string{"macaddress": strings.ToLower(h.WANMAC)}, "set-name": "wan0", "dhcp4": true, "dhcp6": false}, "lan": map[string]any{"match": map[string]string{"macaddress": strings.ToLower(h.LANMAC)}, "set-name": "lan0", "dhcp4": false, "dhcp6": false, "addresses": []string{fmt.Sprintf("%s/%d", helperIP, lan.Bits()), c.Network.APIVIP + "/32", c.Network.IngressVIP + "/32"}}}}
	meta := map[string]any{"instance-id": "okub-" + c.Metadata.Name + "-fedora-helper-1", "local-hostname": h.VMName}
	ub, _ := yaml.Marshal(user)
	ub = append([]byte("#cloud-config\n"), ub...)
	mb, _ := yaml.Marshal(meta)
	nb, _ := yaml.Marshal(net)
	for _, data := range [][]byte{ub, mb, nb} {
		var value any
		if yaml.Unmarshal(data, &value) != nil {
			return nil, nil, nil, errors.New("generated cloud-init is invalid YAML")
		}
	}
	return ub, mb, nb, nil
}

const helperSetupScript = `#!/bin/bash
set -euo pipefail
dnf -y install dnsmasq firewalld haproxy nginx policycoreutils-python-utils
install -d -m 0755 /srv/okub/http /srv/okub/tftp
sysctl --system
semanage fcontext -a -t httpd_sys_content_t '/srv/okub/http(/.*)?'
semanage fcontext -a -t tftpdir_t '/srv/okub/tftp(/.*)?'
semanage port -a -t http_port_t -p tcp 6443
semanage port -a -t http_port_t -p tcp 22623
setsebool -P haproxy_connect_any 1
restorecon -RF /etc/dnsmasq.d /etc/haproxy /etc/nginx
restorecon -RF /srv/okub
dnsmasq --test -C /etc/dnsmasq.d/okub.conf
haproxy -c -f /etc/haproxy/haproxy.cfg
nginx -t -c /etc/nginx/nginx.conf
firewall-offline-cmd --zone=external --change-interface=wan0
firewall-offline-cmd --zone=external --add-masquerade
firewall-offline-cmd --new-policy=okub-lan-wan
firewall-offline-cmd --policy=okub-lan-wan --add-ingress-zone=internal
firewall-offline-cmd --policy=okub-lan-wan --add-egress-zone=external
firewall-offline-cmd --policy=okub-lan-wan --set-target=ACCEPT
firewall-offline-cmd --zone=internal --change-interface=lan0
firewall-offline-cmd --zone=internal --add-source=@LAN_CIDR@
firewall-offline-cmd --zone=internal --add-service=dns
firewall-offline-cmd --zone=internal --add-service=dhcp
firewall-offline-cmd --zone=internal --add-service=tftp
firewall-offline-cmd --zone=internal --add-port=8080/tcp
firewall-offline-cmd --zone=internal --add-port=6443/tcp
firewall-offline-cmd --zone=internal --add-port=22623/tcp
firewall-offline-cmd --zone=internal --add-service=http
firewall-offline-cmd --zone=internal --add-service=https
firewall-offline-cmd --check-config
systemctl enable --now firewalld.service dnsmasq.service haproxy.service nginx.service
`

func dhcpRange(prefix netip.Prefix, c domain.ClusterDefinition) (netip.Addr, netip.Addr) {
	used := map[netip.Addr]bool{prefix.Addr(): true, prefix.Addr().Next(): true, lastHelperAddress(prefix): true, lastHelperAddress(prefix).Prev(): true}
	for _, s := range []string{c.Network.APIVIP, c.Network.IngressVIP, c.Network.RendezvousIP} {
		if a, e := netip.ParseAddr(s); e == nil {
			used[a] = true
		}
	}
	for _, h := range c.Hosts {
		if p, e := netip.ParsePrefix(h.Address); e == nil {
			used[p.Addr()] = true
		}
	}
	var bestStart, bestEnd, runStart netip.Addr
	bestLen, runLen := 0, 0
	for a := prefix.Addr().Next(); prefix.Contains(a) && a != lastHelperAddress(prefix); a = a.Next() {
		if used[a] {
			runLen = 0
			runStart = netip.Addr{}
			continue
		}
		if runLen == 0 {
			runStart = a
		}
		runLen++
		if runLen > bestLen {
			bestStart, bestEnd, bestLen = runStart, a, runLen
		}
	}
	return bestStart, bestEnd
}

func lastHelperAddress(prefix netip.Prefix) netip.Addr {
	b := prefix.Masked().Addr().As4()
	v := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	v |= uint32((uint64(1) << (32 - prefix.Bits())) - 1)
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

func writePrivateAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".render-*")
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
