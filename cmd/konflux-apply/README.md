# Konflux Apply

The Konflux manifests are applied automatically via the [konflux-apply](../../.github/workflows/apply-konflux-manifests.yaml) Github Workflow.

For this workflow, we use a token for Konflux setup as described in [the docs](https://gitlab.cee.redhat.com/konflux/docs/users/-/blob/main/topics/getting-started/getting-access.md#logging-to-the-internal-cluster-with-a-token), with the following service account, role & rolebindings (those manifest can be found in [./manifests](./manifests) too):

1. Service account `gh-action`:
    ```
    apiVersion: v1
    kind: ServiceAccount
    metadata:
      namespace: ocp-serverless-tenant
      name: gh-action
    ```
2. Role with minimal permissions to apply the Konflux manifests:
    ```
    apiVersion: rbac.authorization.k8s.io/v1
    kind: Role
    metadata:
      namespace: ocp-serverless-tenant
      name: gh-action-runner
    rules:
    - apiGroups:
      - appstudio.redhat.com
      resources:
      - applications
      - components
      - imagerepositories
      verbs:
      - get
      - list
      - watch
      - update
      - patch
      - create
      ```
3. Rolebinding
    ```
    apiVersion: rbac.authorization.k8s.io/v1
    kind: RoleBinding
    metadata:
      namespace: ocp-serverless-tenant
      name: gh-action
    subjects:
    - kind: ServiceAccount
      name: gh-action
      apiGroup: ""
    roleRef:
      kind: Role
      name: gh-action-runner
      apiGroup: rbac.authorization.k8s.io
    ```
4. Use the token from this Service Account as the `KONFLUX_SA_TOKEN` repository secret used by the Github workflow
    ```
    kubectl create token gh-action --duration $((6*30*24))h
    ```