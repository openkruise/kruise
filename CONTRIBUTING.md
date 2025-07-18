# Contributing to Openkruise

Welcome to Openkruise! Openkruise consists of several repositories under the organization.
We encourage you to help out by reporting issues, improving documentation, fixing bugs, or adding new features.
Please also take a look at our code of conduct, which details how contributors are expected to conduct themselves as part of the Openkruise community.

## Reporting issues

To be honest, we regard every user of Openkruise as a very kind contributor.
After experiencing Openkruise, you may have some feedback for the project.
Then feel free to open an issue.

There are a lot of cases when you could open an issue:

- bug report
- feature request
- performance issues
- feature proposal
- feature design
- help wanted
- doc incomplete
- test improvement
- any questions on the project
- and so on

Also, we must remind you that when filing a new issue, please remember to remove the sensitive data from your post.
Sensitive data could be passwords, secret keys, network locations, private business data, and so on.

## Code and doc contribution

Every action to make Openkruise better is encouraged.
On GitHub, every improvement for Openkruise could be via a PR (short for pull request).

- If you find a typo, try to fix it!
- If you find a bug, try to fix it!
- If you find some redundant codes, try to remove them!
- If you find some test cases missing, try to add them!
- If you could enhance a feature, please DO NOT hesitate!
- If you find code implicit, try to add comments to make it clear!
- If you find code ugly, try to refactor that!
- If you can help to improve documents, it could not be better!
- If you find document incorrect, just do it and fix that!
- ...

### Workspace Preparation

To put forward a PR, we assume you have registered a GitHub ID.
Then you can finish the preparation in the following steps:

1. **Fork** Fork the repository you wish to work on. You just need to click the button Fork in the right-left of the project repository main page. Then you will end up with your repository in your GitHub username.
2. **Clone** your own repository to develop locally. Use `git clone https://github.com/<your-username>/<project>.git` to clone the repository to your local machine. Then you can create new branches to finish the change you wish to make.
3. **Set remote** upstream to be `https://github.com/openkruise/<project>.git` using the following two commands:

```bash
cd <project>
git remote add upstream https://github.com/openkruise/<project>.git
git remote set-url --push upstream no-pushing
```

Adding this, we can easily synchronize local branches with upstream branches.

4. **Create a branch** to add a new feature or fix issues

Update the local working directory:

```bash
cd <project>
git fetch upstream
git checkout master
git rebase upstream/master
```

Create a new branch:

```bash
git checkout -b <new-branch>
```

Make any change on the new-branch then build and test your codes.

### PR Description

PR is the only way to make changes to Kruise project files.
To help reviewers better understand your purpose, PR description could not be too detailed.
We encourage contributors to follow the [PR template](./.github/PULL_REQUEST_TEMPLATE.md) to finish the pull request.

### Developing Environment

As a contributor, if you want to make any contribution to the Kruise project, we should reach an agreement on the version of tools used in the development environment.
Here are some dependencies with specific versions:

- Golang : v1.22+
- Kubernetes: v1.16+

### Developing guide

There's a `Makefile` in the root folder which describes the options to build and install. Here are some common ones:

```bash
# Generate code and manifests e.g. CRD, RBAC YAML files etc
make manifests

# Build the controller manager binary
make build

# Run the unit tests
make test
```

**There are some guide documents for contributors in [./docs/contributing/](./docs/contributing), such as a debug guide to help you test your own branch in a Kubernetes cluster.**

### Proposals

If you are going to contribute a feature with a new API or need significant effort, please submit a proposal in [./docs/proposals/](./docs/proposals) first.

### Kruise Helm Charts
[kruise charts](https://github.com/openkruise/charts) is the openKruise charts repo, including kruise, kruise rollout, and kruise game.
You can add the corresponding charts package in the versions directory as follows:
```
 versions
 - kruise-game
 - kruise-rollout
 - kruise-state-metrics
 - kruise
   - 1.5.0
   - 1.5.1
   - 1.6.0
   - 1.6.1
```

**make generate_helm_crds** automatically generates crds files under the bin/ directory, which in turn simplifies the generation of helm charts.

## Engage to help anything

We choose GitHub as the primary place for Openkruise to collaborate.
So the latest updates of Openkruise are always here.
Although contributions via PR are an explicit way to help, we still call for any other ways.

- reply to other's issues if you could;
- help solve other user's problems;
- help review other's PR design;
- help review other's codes in PR;
- discuss Openkruise to make things clearer;
- advocate Openkruise technology beyond GitHub;
- write blogs on Openkruise and so on.

In a word, **ANY HELP IS CONTRIBUTION**.

## Join Openkruise as a member

It is also welcomed to join the Openkruise team if you are willing to participate in the Openkruise community continuously and keep active.
Please read and follow the [Community Membership](https://github.com/openkruise/community/blob/master/community-membership.md).
