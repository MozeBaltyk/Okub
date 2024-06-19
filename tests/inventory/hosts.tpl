# Generated with hosts.tpl
[all]
## ALL HOSTS
localhost ansible_connection=local

[OKD_BOOTSTRAP]
%{ for idx, ip in bootstrap_ips ~}
okd-bootstrap0${idx + 1} ansible_host=${ip}
%{ endfor ~}
