set shell := ["bash", "-uc"]

# Test if oc-mirror is already installed or install it if not already
_oc_mirror_availability:
    #!/usr/bin/env bash
    set -e
    if command -v oc-mirror >/dev/null 2>&1; then
        printf "\e[1;34mINFO\e[m: oc-mirror \e[1;32mis already installed.\e[m \n\n"
        oc-mirror version --output=yaml
    else
        printf "\e[1;33mINFO\e[m: oc-mirror was not found. Installing it...\n"
        curl -k https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/latest/oc-mirror.tar.gz -o $HOME/.local/bin/oc-mirror.tar.gz
        tar xzf $HOME/.local/bin/oc-mirror.tar.gz -C $HOME/.local/bin/oc-mirror
        chmod +x $HOME/.local/bin/oc-mirror
        oc-mirror version --output=yaml
    fi

# Create an archive of all images from given namespace with oc-mirror
from_ns namespace storepath:
    @just _oc_mirror_availability

    #!/usr/bin/env bash
    set -e
    # Create base dir for package
    printf "\e[1;34mINFO\e[m: Create directory {{storepath}}/{{namespace}} \n\n"
    mkdir -p {{storepath}}/{{namespace}}

    # Put oc-mirror inside the package
    printf "\e[1;34mINFO\e[m: Download oc-mirror to {{storepath}}/{{namespace}} \n\n"
    curl -k https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/latest/oc-mirror.tar.gz -o {{storepath}}/{{namespace}}/oc-mirror.tar.gz
    printf "\e[1;34mINFO\e[m: untar oc-mirror to {{storepath}}/{{namespace}} \n\n"
    tar xzf {{storepath}}/{{namespace}}/oc-mirror.tar.gz -C {{storepath}}/{{namespace}}
    chmod +x {{storepath}}/{{namespace}}/oc-mirror
    rm -f {{storepath}}/{{namespace}}/oc-mirror.tar.gz

    # Get images
    list_images=()
    images=$(oc get pods -n {{namespace}} --output jsonpath='{range .items[*]}{.spec.containers[*].image}{"\n"}{end}')
    for item in ${images}; do list_images+=($item) ; done
    printf "\e[1;34mINFO\e[m: List images \n\n"
    printf '%s\n' "${list_images[@]}" | jq -R . | jq -s .
    printf "\n"

    #Convert in json
    list_images_json=$(jq -nc '{list_images: $ARGS.positional}' --args ${list_images[@]})

    # Get registries
    list_registries=()
    registries=$( for i in ${images}; do echo ${i%%/*}; done | uniq )
    for item in $(echo ${registries}); do list_registries+=($item) ; done
    printf "\e[1;34mINFO\e[m: List registries \n\n"
    for array in ${list_registries[@]}; do echo $array ; done
    printf "\n"

    # Login podman to registry
    for item in ${list_registries[@]}; do
      printf "\e[1;34mINFO\e[m: Login registries ${item} \n\n"
      podman login ${item} --tls-verify=false
      printf "\n"
    done

    # Push template
    printf "\e[1;34mINFO\e[m: Create directory {{storepath}}/{{namespace}} \n\n"
    mkdir -p {{storepath}}/{{namespace}}
    echo "${list_images_json}" > {{storepath}}/{{namespace}}/images.json

    printf "\e[1;34mINFO\e[m: Push Template {{storepath}}/{{namespace}}/imageset-config.yaml \n\n"
    ansible all -i "localhost," --connection=local -m template -a "src=templates/imageset.yaml.j2 dest={{storepath}}/{{namespace}}/imageset-config.yaml" -e "@{{storepath}}/{{namespace}}/images.json"

    # Mirror locally
    printf "\e[1;34mINFO\e[m: Mirror all images in {{storepath}}/{{namespace}} \n\n"
    oc-mirror --config={{storepath}}/{{namespace}}/imageset-config.yaml file://{{storepath}}/{{namespace}}/

    # TAR package 
    printf "\e[1;34mINFO\e[m: Tar {{storepath}}/{{namespace}} \n\n"
    tar czfP {{storepath}}/{{namespace}}.tar.gz {{storepath}}/{{namespace}}

# upload images to registry with oc-mirror from given archive
upload registry org package:
    @just _oc_mirror_availability

    #!/usr/bin/env bash
    set -e
    tar_to_unpack="{{package}}"
    path_to_unpack="${tar_to_unpack%%.*}"

    # UnTAR package 
    printf "\e[1;34mINFO\e[m: Untar ${tar_to_unpack} \n\n"
    mkdir -p ${path_to_unpack}
    tar xzfP ${tar_to_unpack} -C ${path_to_unpack}

    # Login target registry
    printf "\e[1;34mINFO\e[m: Login registries {{registry}} \n\n"
    podman login {{registry}} --tls-verify=false
    printf "\n"

    ## can be done with yq 
    # Add storageConfig to imageset.yml
    printf "\e[1;34mINFO\e[m: Add blockinfile to imageset-config.yaml\n\n"
    storageConfig="storageConfig:
      local:
        path: ${path_to_unpack}"

    if ! grep -q storageConfig ${path_to_unpack}/imageset-config.yaml; then 
      echo -e "$( head -2 ${path_to_unpack}/imageset-config.yaml )\n${storageConfig}\n$( tail -n +3 ${path_to_unpack}/imageset-config.yaml )" > ${path_to_unpack}/imageset-config.yaml
    fi

    #Upload images
    printf "\e[1;34mINFO\e[m: Upload images in registry {{registry}} \n\n"
    ${path_to_unpack}/oc-mirror --config=${path_to_unpack}/imageset-config.yaml docker://{{registry}}/{{org}} --dest-skip-tls