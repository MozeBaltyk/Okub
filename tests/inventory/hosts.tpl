# Generated with hosts.tpl
[all]
## ALL HOSTS
localhost ansible_connection=local

[TEST1]
%{ for idx, ip in test1_ips ~}
test1_${idx} ansible_host=${ip} # test1_${idx}
%{ endfor ~}

[TEST2]
%{ for idx, ip in test2_ips ~}
test2_${idx} ansible_host=${ip} # test2_${idx}
%{ endfor ~}

[TEST:children]
TEST1
TEST2
