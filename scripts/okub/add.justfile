set shell := ["bash", "-uc"]

# https://docs.openshift.com/container-platform/4.15/nodes/nodes/nodes-sno-worker-nodes.html#sno-adding-worker-nodes-to-single-node-clusters-manually_add-workers
# oc extract -n openshift-machine-api secret/worker-user-data-managed --keys=userData --to=- > worker.ign