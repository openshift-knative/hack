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
