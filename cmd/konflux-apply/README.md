# Konflux Apply

The Konflux manifests are applied automatically via the [konflux-apply](../../.github/workflows/apply-konflux-manifests.yaml) Github Workflow.

For this workflow, we use a token for Konflux setup as described in [the docs](https://konflux.pages.redhat.com/docs/users/getting-started/getting-access.html#accessing-konflux-via-cli). Therefor we need to setup a service account, role & rolebinding:

1. Service account `gh-action`:
    ```shell
    kubectl apply -f ./cmd/konflux-apply/manifests/gh-action-serviceaccount.yaml
    ```
2. Role with minimal permissions to apply the Konflux manifests:
    ```shell
    kubectl apply -f ./cmd/konflux-apply/manifests/gh-action-role.yaml
      ```
3. Rolebinding
    ```shell
    kubectl apply -f ./cmd/konflux-apply/manifests/gh-action-rolebinding.yaml
    ```
4. Use the token from the `gh-action` Service account as the `KONFLUX_SA_TOKEN` repository secret used by the Github workflow
    ```shell
    kubectl create token gh-action --duration $((6*30*24))h
    ```
   
## Revoke and Recreate Token

As we use by default Tokens with a validity of 6 months, we need to recreate them periodically. This is done via the following:

1. Make sure, you're logged in to Konflux via the CLI and have access to the `ocp-serverless` workspace. Check for example the [Konflux kickstart recording](https://drive.google.com/drive/u/0/folders/0AB3Zk0vHI6ulUk9PVA) or the [Konflux docs](https://gitlab.cee.redhat.com/konflux/docs/users/-/blob/main/topics/getting-started/getting-access.md#accessing-konflux-via-cli)
2. Recreate the `gh-action` service account:
    ```shell
   kubectl delete -f https://raw.githubusercontent.com/openshift-knative/hack/main/cmd/konflux-apply/manifests/gh-action-serviceaccount.yaml
   kubectl apply -f https://raw.githubusercontent.com/openshift-knative/hack/main/cmd/konflux-apply/manifests/gh-action-serviceaccount.yaml
   ```
3. Create a new Token for 6 months and [update the `KONFLUX_SA_TOKEN` secret](https://github.com/openshift-knative/hack/settings/secrets/actions) with its value:
   ```shell
   kubectl create token gh-action --duration $((6*30*24))h
   ```