# hack

CI tooling and hacks to improve CI

## Generate openshift/release config

- Add configuration for your repository in `config/<file.yaml>`
    - If you're adding a new file in `config/` directory, add the new file to the `make generate-ci`
      command
- Run `make generate-ci REMOTE=<your_remote>`
    - For example, `make generate-ci REMOTE=git@github.com:pierDipi/release.git`
- Create a PR to [https://github.com/openshift/release](https://github.com/openshift/release) (to be
  automated)

## Run unit tests

```shell
make unit-tests
```

## Updating OpenShift versions

CI configs use specific OpenShift versions. To change the version, you need to update the YAML files in the `config/` directory.

When a new OpenShift version is released, wait until the cluster pool for OpenShift CI is available in 
[https://docs.ci.openshift.org/docs/how-tos/cluster-claim/#existing-cluster-pools](https://docs.ci.openshift.org/docs/how-tos/cluster-claim/#existing-cluster-pools).
