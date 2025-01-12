# Generated with hosts.tpl
[all]
## ALL HOSTS
localhost ansible_connection=local

[OKUB_HELPER]
%{ for idx, ip in helper_ips ~}
${helper_hostname}${idx + 1}.${domain} ansible_host=${ip}
%{ endfor ~}

[OKUB_BOOTSTRAP]
bootstrap01.${domain} 

[OKUB_MASTERS]
%{ for idx in range(masters) ~}
master${idx + 1}.${domain} # Master${idx + 1}
%{ endfor ~}

[OKUB_WORKERS]
%{ for idx in range(workers) ~}
worker${idx + 1}.${domain} # Worker${idx + 1}
%{ endfor ~}

[OKUB_CLUSTER:children]
OKUB_BOOTSTRAP
OKUB_MASTERS
OKUB_WORKERS
