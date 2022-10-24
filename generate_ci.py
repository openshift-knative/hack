#!/usr/bin/env python3
import argparse
import copy
import dataclasses
import os
import re
import shutil
from os import listdir
from os.path import isfile, join
from subprocess import run as shrun, PIPE

# Assumptions with the idea of creating conventions over configurations.

# 1. Dockerfiles are self-contained, and they don't require any binary
#    prebuilt on the host
# 2. Dockerfiles are in the directory matching
#    `openshift/ci-operator/knative-images/**/Dockerfile`
#    or openshift/ci-operator/knative-test-images/**/Dockerfile
# 3. The repository has a Makefile in the root and has some targets containing the keyword
#    `test`

# Usage:
# ./generate_ci.py \
#   --openshift-release-remove git@github.com:pierDipi/release.git # Remote to your fork

openshift_release_remote = "git@github.com:pierDipi/release.git"

parser = argparse.ArgumentParser(description='Generate a test case')
parser.add_argument('--openshift-release-remote', type=str, dest='openshift_release_remote',
                    default=openshift_release_remote,
                    help=f"Openshift release remote URL: {openshift_release_remote}")
args = parser.parse_args()

openshift_release_remote = args.openshift_release_remote

ocp_min_version_key = "ocp_min_version"
ocp_max_version_key = "ocp_max_version"

repos = {
    "openshift/knative-eventing": {
        # Every test target contains `test` in the name and matches
        # one of the following regex.
        "e2e": {
            "match": [
                ".*e2e$",
                ".*reconciler.*",
                ".*conformance.*",
            ],
            "reporter": {
                "slack": {
                    "channel": "#knative-eventing-ci"
                }
            }
        },
        "images": {
            # Optional prefix
            # Default to repository name
            "prefix": "knative-eventing"
        },
        "branch": {
            # Additional supported branches
            "additional": {
                "release-v1.6": {
                    ocp_min_version_key: "4.8",
                    ocp_max_version_key: "4.11",
                },
            }
        }
    },
    # TODO serving version branches, they have minor versions should we align those to be floating?
    # TODO Self-contained dockerfiles as in https://github.com/openshift/knative-eventing/pull/1785
    # "openshift/knative-serving": {
    #     # Every test target contains `test` in the name and matches
    #     # one of the following regex.
    #     "e2e": {
    #         "match": [
    #             ".*e2e$",
    #             ".*reconciler.*",
    #             ".*conformance.*",
    #         ]
    #     },
    #     "images": {
    #         # Optional prefix
    #         # Default to repository name
    #         "prefix": "knative-serving"
    #     },
    #     "supported_branches": {
    #         "release-v1.1.2",
    #         "release-v1.2.0",
    #         "release-v1.3",
    #         "release-next"
    #     }
    # },
    "openshift-knative/eventing-kafka-broker": {
        "e2e": {
            "match": [
                ".*e2e$",
                ".*reconciler.*",
                ".*conformance.*",
            ],
            "reporter": {
                "slack": {
                    "channel": "#knative-eventing-ci"
                }
            }
        },
        "images": {
            "prefix": "knative-eventing-kafka-broker"
        }
    },
}

supported_branches = {
    "release-v1.4": {
        ocp_min_version_key: "4.6",
        ocp_max_version_key: "4.11",
    },
    "release-v1.5": {
        ocp_min_version_key: "4.8",
        ocp_max_version_key: "4.11",
    },
    "release-next": {
        ocp_min_version_key: "4.8",
        ocp_max_version_key: "4.11",
    }
}

openshift_release = "openshift/release"
openshift_release_ci_config = openshift_release + "/ci-operator/config"
openshift_release_ci_jobs = openshift_release + "/ci-operator/jobs"
openshift_release_image_mirroring = f"{openshift_release}/core-services/image-mirroring/knative"
openshift_release_registry_namespace = "openshift"
openshift_release_registry = f"registry.ci.openshift.org/{openshift_release_registry_namespace}"

quay_registry = "quay.io/openshift-knative"


def run(*args, **kwargs):
    result = shrun(*args, **kwargs)
    print(result.stdout)
    print(result.stderr)
    assert result.returncode == 0
    return result


def checkout_branch(name, branch):
    print(f"Checkout branch {branch}")
    run(f"git checkout {branch}", stdout=PIPE, stderr=PIPE, universal_newlines=True,
        shell=True, cwd=name)


def clone_repository(name: str, def_branch="main"):
    print(f"Clone repository {name}")
    run(f"git clone --mirror https://github.com/{name}.git {name}/.git", stdout=PIPE, stderr=PIPE,
        universal_newlines=True, shell=True)
    run("git config --bool core.bare false", stdout=PIPE, stderr=PIPE, universal_newlines=True,
        shell=True, cwd=name)
    checkout_branch(name, def_branch)


def filter_targets(r, targets):
    new_targets = []
    print("Original targets", targets)
    for t in targets:
        if len(t) == 0:
            continue
        t = t.rstrip("\t")
        t = t.rstrip(" ")
        t = t.rstrip(":")
        regex = r["e2e"]["match"]

        print("Matching target", t, "with regex", regex)

        for m in regex:
            if re.compile(m).match(t):
                new_targets.append(t)
                break

    return new_targets


def get_test_targets(name, r):
    tests = run(f"cat {name}/Makefile | grep ^.*test.*:", stdout=PIPE, stderr=PIPE, universal_newlines=True,
                shell=True)
    return filter_targets(r, tests.stdout.split('\n'))


def get_images(path, name, r, prefix_ctx=""):
    images = []
    root = os.path.join(name, path)
    directories = [x[0] for x in os.walk(root)]

    if len(directories) <= 1:
        return root

    for d in directories:
        if d == root:
            continue
        image_name = os.path.basename(d)
        if r["images"]["prefix"] is None:
            prefix = name[name.index("/") + 1:]
        else:
            prefix = r["images"]["prefix"]

        image_name = prefix + "-" + prefix_ctx + image_name
        images.append(Image(
            name=image_name.lower().replace("_", "-"),
            env_name=image_name.upper().replace("-", "_"),
            path=d.lstrip(name),
        ))

    return sorted(images)


for name, r in repos.items():
    directory = name[:name.index("/")]
    shutil.rmtree(directory, ignore_errors=True)
    os.makedirs(directory, exist_ok=True)

shutil.rmtree(openshift_release[:openshift_release.index("/")], ignore_errors=True)
os.makedirs(openshift_release[:openshift_release.index("/")], exist_ok=True)

for name, r in repos.items():
    clone_repository(name)

clone_repository(openshift_release, def_branch="master")


@dataclasses.dataclass
class Image:
    name: str
    env_name: str
    path: str

    def __lt__(self, other):
        return self.name < other.name


for name, r in repos.items():

    org = name[:name.index('/')]
    repo_name = name[name.index('/') + 1:]

    branches = copy.deepcopy(supported_branches)
    if r.get("branch") is not None and r["branch"].get("additional") is not None:
        for b, v in r["branch"]["additional"].items():
            branches[b] = v

    # TODO: for now we're reconciling only supported branches, eventually we need to reconcile the entire
    #  directory to make sure that we delete periodic jobs from unsupported branches
    for supported_branch in branches:
        run(f"rm -rf {openshift_release_ci_config}/{name}/*{supported_branch}*.yaml", stdout=PIPE, stderr=PIPE,
            universal_newlines=True, shell=True)

    for supported_branch, openshift_versions in branches.items():

        for _, ov in openshift_versions.items():

            if supported_branch == "release-next":
                promotion_name = "knative-nightly"
            else:
                promotion_name = supported_branch.replace("release", "knative")

            print(f"=== {name} ===")
            checkout_branch(name, supported_branch)

            test_targets = get_test_targets(name, r)
            print("---> Matching targets", test_targets)

            build_image = "openshift/ci-operator/build-image"
            print("---> Build image", build_image)

            images = get_images(path="openshift/ci-operator/knative-images", name=name, r=r)
            print("---> Images", images)
            test_images = get_images(path="openshift/ci-operator/knative-test-images", name=name, r=r,
                                     prefix_ctx="test-")
            print("---> Test images", test_images)

            mirroring_lines = []
            for img in images + test_images:
                frm = f"{openshift_release_registry}/{promotion_name}:{img.name}"
                to = f"{quay_registry}/{img.name}:{promotion_name}"
                mirroring_lines.append(f"{frm} {to}\n")

            file_name = f"mapping_{promotion_name}_{repo_name}_quay"
            with open(f"{openshift_release_image_mirroring}/{file_name}", "w") as f:
                f.writelines(mirroring_lines)

            images_field = ""
            dependencies = ""
            for img in images + test_images:
                images_field += f"""
- dockerfile_path: openshift/{img.path}/Dockerfile
  to: {img.name}"""

                dependencies += f"""
      - env: {img.env_name}
        name: {img.name}"""

            tests_field = ""
            continuous = ""
            schedule = ""

            for x in range(2):
                for test_target in test_targets:
                    tests_field += f"""
- as: {test_target}-aws-ocp-{ov.replace('.', '')}{continuous}
  cluster_claim:
    architecture: amd64
    cloud: aws
    owner: openshift-ci
    product: ocp
    timeout: 1h0m0s
    version: "{ov}"
  {schedule}
  steps:
    test:
    - as: test
      cli: latest
      commands: make {test_target}
      dependencies: {dependencies}
      from: src
      resources:
        requests:
          cpu: 100m
      timeout: 4h0m0s
    workflow: generic-claim
            """
                continuous = "-continuous"
                schedule = "cron: 0 5 * * 2,6"

            ci = f""" # Generated by generate_ci.py
base_images:
  base:
    name: "{ov}"
    namespace: ocp
    tag: base
build_root:
  project_image:
    dockerfile_path: {build_image}/Dockerfile
images: {images_field}
promotion:
  additional_images:
    {name[name.index("/") + 1:]}-src: src
  disabled: {"true" if openshift_versions[ocp_max_version_key] != ov else "false"}
  name: {promotion_name}
  namespace: {openshift_release_registry_namespace}
releases:
  initial:
    integration:
      name: "{ov}"
      namespace: ocp
  latest:
    integration:
      include_built_images: true
      name: "{ov}"
      namespace: ocp
resources:
  '*':
    requests:
      cpu: 500m
      memory: 1Gi
tests: {tests_field}
zz_generated_metadata:
  branch: {supported_branch}
  org: {org}
  repo: {repo_name}
  variant: "{ov.replace('.', '')}"
    """
            dir = f"{openshift_release_ci_config}/{name}"
            with open(f"{dir}/{org}-{repo_name}-{supported_branch}__{ov.replace('.', '')}.yaml", "w") as f:
                f.write(ci)

run("make jobs ci-operator-config", stdout=PIPE, stderr=PIPE, universal_newlines=True,
    shell=True, cwd=openshift_release)

# Add periodics reporter
for name, r in repos.items():
    reporter = f"""
- agent: kubernetes
  reporter_config:
    slack:
      channel: '{r['e2e']['reporter']['slack']['channel']}'
      job_states_to_report:
      - success
      - failure
      - error
      report_template: '{{{{if eq .Status.State "success"}}}} :rainbow: Job *{{{{.Spec.Job}}}}* ended with *{{{{.Status.State}}}}*. <{{{{.Status.URL}}}}|View logs> :rainbow: {{{{else}}}} :volcano: Job *{{{{.Spec.Job}}}}* ended with *{{{{.Status.State}}}}*. <{{{{.Status.URL}}}}|View logs> :volcano: {{{{end}}}}'
    """

    dir = f"{openshift_release_ci_jobs}/{name}"
    periodics_tests = [join(dir, f) for f in listdir(dir) if isfile(join(dir, f)) and "periodics" in f]
    print(f"Periodic tests in {dir}", periodics_tests)
    for periodics_test in periodics_tests:
        print(f"Adding reporter to {periodics_test}")
        with open(periodics_test, "r") as f:
            content = f.read()
        content = content.replace("- agent: kubernetes", reporter)

        with open(periodics_test, "w") as f:
            f.write(content)

# Reformat CI config with the reporter
run("make ci-operator-config jobs", stdout=PIPE, stderr=PIPE, universal_newlines=True,
    shell=True, cwd=openshift_release)

# Restore Makefile changes
run(f"git restore Makefile", stdout=PIPE, stderr=PIPE, universal_newlines=True,
    shell=True, cwd=openshift_release)
# Commit changes
branch = "sync-serverless-ci"
run(f"git checkout -b {branch}", stdout=PIPE, stderr=PIPE, universal_newlines=True,
    shell=True, cwd=openshift_release)
run("git add .", stdout=PIPE, stderr=PIPE, universal_newlines=True,
    shell=True, cwd=openshift_release)
run("git commit -m 'Sync Serverless CI'", stdout=PIPE, stderr=PIPE, universal_newlines=True,
    shell=True, cwd=openshift_release)
run(f"git remote add fork {openshift_release_remote}", stdout=PIPE, stderr=PIPE, universal_newlines=True,
    shell=True, cwd=openshift_release)
run(f"git push fork {branch} -f", stdout=PIPE, stderr=PIPE, universal_newlines=True,
    shell=True, cwd=openshift_release)

# gh pr create --fill --no-maintainer-edit --base master
