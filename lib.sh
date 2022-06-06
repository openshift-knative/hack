# scale up worker nodes to at least the provided number of nodes
# Inputs:
#  1: number of minimum worker nodes
function scale_worker_nodes() {
  num_workers=${1:?Pass num_workers as arg[1]}

  echo "Reconcile workers to at least ${num_workers} nodes"

  additional_replicas=$(oc get machineset -n openshift-machine-api | awk '{print $2}' | tail -n +2 | awk -v workers="${num_workers}" '{sum+=$1} END {print workers-sum}')
  echo "Additional replicas ${additional_replicas}"

  if [[ ${additional_replicas} -gt 0 ]]; then
    machineset="$(oc get machineset -n openshift-machine-api | awk '{print $1}' | tail -n +2 | head -1)"
    replicas=$(oc get machineset -n openshift-machine-api "${machineset}" -o=jsonpath='{.spec.replicas}')
    replicas=$(expr ${replicas} + ${additional_replicas})
    oc scale machineset "${machineset}" -n openshift-machine-api --replicas="${replicas}"
    wait_for_machine_set_to_be_ready "${machineset}"
  fi
}

# Waits for the provided machineset to have all expected replicas ready
# Inputs:
#  1: machineset name
function wait_for_machine_set_to_be_ready() {
  machineset_name=${1:?Pass machineset_name as arg[1]}
  replicas=$(oc get machineset -n openshift-machine-api "${machineset_name}" -o=jsonpath='{.spec.replicas}')
  oc wait machineset "${machineset}" -n openshift-machine-api --for=jsonpath='{.status.readyReplicas}'="${replicas}" --timeout=30m
}

