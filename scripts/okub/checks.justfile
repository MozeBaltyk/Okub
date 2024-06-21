set shell := ["bash", "-uc"]

# Check DNS
DNS:
    #!/usr/bin/env bash
    echo "DNS"
    dig +noall +answer @<nameserver_ip> console-openshift-console.apps.<cluster_name>.<base_domain>

# Check DHCP
DHCP:
    #!/usr/bin/env bash
    echo "DHCP"

# Check LOADBALANCER
LOADBALANCER:
    #!/usr/bin/env bash
    echo "LOADBALANCER"

# Check FIREWALL
FIREWALL:
    #!/usr/bin/env bash
    echo "FIREWALL"

# Checks all
ALL:
    #!/usr/bin/env bash
    just -f checks.justfile DNS
    just -f checks.justfile DHCP
    just -f checks.justfile LOADBALANCER
    just -f checks.justfile FIREWALL