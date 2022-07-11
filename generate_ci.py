#!/usr/bin/env python3
import argparse
import os
import re
import shutil
from os import listdir
from os.path import isfile, join
from subprocess import run, PIPE

# Assumptions with the idea of creating conventions over configurations.

# 1. Dockerfiles are self-contained, and they don't require any binary
#    prebuilt on the host
# 2. Dockerfiles are in the directory matching
#    `openshift/ci-operator/knative-images/**/Dockerfile`
#    or openshift/ci-operator/knative-test-images/**/Dockerfile
# 3. The repository has a Makefile in the root and has some targets containing the keyword
#    `test`

openshift_release_remote = "git@github.com:pierDipi/release.git"

parser = argparse.ArgumentParser(description='Generate a test case')
parser.add_argument('--openshift-release-remote', type=str, dest='openshift_release_remote',
                    default=openshift_release_remote,
                    help=f"Openshift release remote URL: {openshift_release_remote}")
args = parser.parse_args()

openshift_release_remote = args.openshift_release_remote

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

# TODO take them from SO main?
#  There might be a delay between having CI cluster pools for a max version and our support version for OCP, so let's
#  not do this until we are sure about getting cluster pools and supported versions.
openshift_versions = [
    "4.6",  # min version
    "4.10"  # max version
]

supported_branches = [
    "release-v1.0",
    "release-v1.1",
    "release-v1.2",
    "release-v1.3",
    "release-v1.4",
    "release-next"
]

openshift_release = "openshift/release"
openshift_release_ci_config = openshift_release + "/ci-operator/config"
openshift_release_ci_jobs = openshift_release + "/ci-operator/jobs"


def checkout_branch(name, branch):
    print(f"Checkout branch {branch}")
    result = run(f"git checkout {branch}", stdout=PIPE, stderr=PIPE, universal_newlines=True,
                 shell=True, cwd=name)
    print(result.stdout)
    print(result.stderr)
    assert result.returncode == 0


def clone_repository(name: str, def_branch="main"):
    print(f"Clone repository {name}")
    result = run(f"git clone --mirror https://github.com/{name}.git {name}/.git", stdout=PIPE, stderr=PIPE,
                 universal_newlines=True, shell=True)
    print(result.stdout)
    print(result.stderr)
    assert result.returncode == 0
    result = run("git config --bool core.bare false", stdout=PIPE, stderr=PIPE, universal_newlines=True,
                 shell=True, cwd=name)
    print(result.stdout)
    print(result.stderr)
    assert result.returncode == 0
    checkout_branch(name, def_branch)


def filter_targets(r, targets):
    new_targets = []
    print("Original targets", targets)
    for t in targets:
        if t == "":
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
    return filter_targets(r, tests.stdout.split("\n"))


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
        images.append((d.lstrip(name), image_name.upper()))

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
result = run(f"git apply ../../Makefile.patch", stdout=PIPE, stderr=PIPE, universal_newlines=True, shell=True,
             cwd=openshift_release)
print(result.stdout)
print(result.stderr)
assert result.returncode == 0

for name, r in repos.items():

    run(f"rm -rf {openshift_release_ci_config}/{name}/*.yaml", stdout=PIPE, stderr=PIPE,
        universal_newlines=True, shell=True)

    for ov in openshift_versions:
        for supported_branch in supported_branches:

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

            images_field = ""
            dependencies = ""
            for img in images + test_images:
                images_field += f"""
- dockerfile_path: openshift/{img[0]}/Dockerfile
  to: {img[1].lower().replace('_', '-')}"""

                dependencies += f"""
      - env: {img[1].replace('-', '_')}
        name: {img[1].lower().replace('_', '-')}"""

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
  disabled: {"true" if openshift_versions[-1] != ov else "false"}
  name: {promotion_name}
  namespace: openshift
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
  org: {name[:name.index('/')]}
  repo: {name[name.index('/') + 1:]}
  variant: "{ov.replace('.', '')}"
    """
            dir = f"{openshift_release_ci_config}/{name}"
            with open(f"{dir}/{name.replace('/', '-')}-{supported_branch}__{ov.replace('.', '')}.yaml", "w") as f:
                f.write(ci)

result = run("make jobs ci-operator-config", stdout=PIPE, stderr=PIPE, universal_newlines=True,
             shell=True, cwd=openshift_release)
assert result.returncode == 0

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
      report_template: '{{if eq .Status.State "success"}} :rainbow: Job *{{.Spec.Job}}* ended with *{{.Status.State}}*. <{{.Status.URL}}|View logs> :rainbow: {{else}} :volcano: Job *{{.Spec.Job}}* ended with *{{.Status.State}}*. <{{.Status.URL}}|View logs> :volcano: {{end}}'
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
result = run("make ci-operator-config jobs", stdout=PIPE, stderr=PIPE, universal_newlines=True,
             shell=True, cwd=openshift_release)
assert result.returncode == 0

# Restore Makefile changes
result = run(f"git restore Makefile", stdout=PIPE, stderr=PIPE, universal_newlines=True,
             shell=True, cwd=openshift_release)
print(result.stdout)
print(result.stderr)
assert result.returncode == 0

# Commit changes
branch = "sync-serverless-ci"
result = run(f"git checkout -b {branch}", stdout=PIPE, stderr=PIPE, universal_newlines=True,
             shell=True, cwd=openshift_release)
print(result.stdout)
print(result.stderr)
assert result.returncode == 0
result = run("git add .", stdout=PIPE, stderr=PIPE, universal_newlines=True,
             shell=True, cwd=openshift_release)
print(result.stdout)
print(result.stderr)
assert result.returncode == 0
result = run("git commit -m 'Sync Serverless CI'", stdout=PIPE, stderr=PIPE, universal_newlines=True,
             shell=True, cwd=openshift_release)
print(result.stdout)
print(result.stderr)
assert result.returncode == 0
result = run(f"git remote add fork {openshift_release_remote}", stdout=PIPE, stderr=PIPE, universal_newlines=True,
             shell=True, cwd=openshift_release)
print(result.stdout)
print(result.stderr)
assert result.returncode == 0
result = run(f"git push fork {branch} -f", stdout=PIPE, stderr=PIPE, universal_newlines=True,
             shell=True, cwd=openshift_release)
print(result.stdout)
print(result.stderr)
assert result.returncode == 0

# gh pr create --fill --no-maintainer-edit --base master
