set shell := ["bash", "-uc"]

# Get Cue
cue:
    #!/usr/bin/env bash
    set -e
    printf "\e[1;34m[INFO]\e[m Install CUElang:\n";    
    podman pull docker.io/cuelang/cue:latest