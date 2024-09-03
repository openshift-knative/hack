# Konflux Apply

The Konflux manifests are applied automatically via the [konflux-apply](../../.github/workflows/apply-konflux-manifests.yaml) Github Workflow.

For this workflow, we use a token for Konflux setup as described in [the docs](https://gitlab.cee.redhat.com/konflux/docs/users/-/blob/main/topics/getting-started/getting-access.md#logging-to-the-internal-cluster-with-a-token). Therefor we need to setup a service account, role & rolebinding:

1. Service account `gh-action`:
    ```
    kubectl apply -f ./manifests/gh-action-serviceaccount.yaml
    ```
2. Role with minimal permissions to apply the Konflux manifests:
    ```
    kubectl apply -f ./manifests/gh-action-role.yaml
      ```
3. Rolebinding
    ```
    kubectl apply -f ./manifests/gh-action-rolebinding.yaml
    ```
4. Use the token from the `gh-action` Service account as the `KONFLUX_SA_TOKEN` repository secret used by the Github workflow
    ```
    kubectl create token gh-action --duration $((6*30*24))h
    ```