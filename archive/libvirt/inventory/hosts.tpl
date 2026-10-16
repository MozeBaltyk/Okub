# Generated with hosts.tpl
[all]
## ALL HOSTS
localhost ansible_connection=local

[OKUB_HELPER]
%{ for idx, ip in helper_ips ~}
${helper_hostname}${idx + 1} ansible_host=${ip}
%{ endfor ~}

[OKUB_MASTERS]
%{ for master in master_details ~}
${master.name} ansible_host=${master.ip} ansible_mac=${master.mac}
%{ endfor ~}

[OKUB_WORKERS]
%{ for worker in worker_details ~}
${worker.name} ansible_host=${worker.ip} ansible_mac=${worker.mac}
%{ endfor ~}

[OKUB_CLUSTER:children]
OKUB_MASTERS
OKUB_WORKERS
