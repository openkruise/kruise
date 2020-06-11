# Contributing to Kruise

Welcome to Kruise!It is warmly welcomed if you have interest to hack on Kruise.

We encourage this kind of willing very much, and here is a list of contributing guide for you.

## Reporting issues

To be honest, we regard every user of Kruise as a very kind contributor.
After experiencing Kruise, you may have some feedback for the project.
Then feel free to open an issue via [NEW ISSUE](https://github.com/openkruise/kruise/issues/new/choose).

There are lot of cases when you could open an issue:

- bug report
- feature request
- performance issues
- feature proposal
- feature design
- help wanted
- doc incomplete
- test improvement
- any questions on project
- and so on

Also we must remind that when filing a new issue, please remember to remove the sensitive data from your post.
Sensitive data could be password, secret key, network locations, private business data and so on.

## Code and doc contribution

Every action to make Kruise better is encouraged.
On GitHub, every improvement for Kruise could be via a PR (short for pull request).

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
Then you could finish the preparation in the following steps:

1. **Fork** Kruise to your repository. You just need to click the button Fork in right-left of openkruise/kruise main page. Then you will end up with your repository in `https://github.com/<your-username>/kruise`, in which `your-username` is your GitHub username.
2. **Clone** your own repository to develop locally. Use `git clone https://github.com/<your-username>/kruise.git` to clone repository to your local machine. Then you can create new branches to finish the change you wish to make.
3. **Set remote** upstream to be `https://github.com/openkruise/kruise.git` using the following two commands:

```bash
git remote add upstream https://github.com/openkruise/kruise.git
git remote set-url --push upstream no-pushing
```

Adding this, we can easily synchronize local branches with upstream branches.

4. **Create a branch** to add a new feature or fix issues

Update local working directory:

```bash
cd kruise
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

PR is the only way to make change to Kruise project files.
To help reviewers better get your purpose, PR description could not be too detailed.
We encourage contributors to follow the [PR template](./.github/PULL_REQUEST_TEMPLATE.md) to finish the pull request.

### Developing Environment

As a contributor, if you want to make any contribution to Kruise project, we should reach an agreement on the version of tools used in the development environment.
Here are some dependents with specific version:

- Golang : v1.13+ (1.14 is best)
- Kubernetes: v1.12+

### Developing guide

There's a `Makefile` in the root folder which describes the options to build and install. Here are some common ones:

```bash
# Build the controller manager binary
make manager

# Run the tests
make test

# Generate manifests e.g. CRD, RBAC YAML files etc
make manifests
```

If you want to start kruise-controller-manager locally to work with a Kubernetes cluster, you should follow the [debug guide](./docs/debug/README.md).

## Engage to help anything

We choose GitHub as the primary place for Kruise to collaborate.
So the latest updates of Kruise are always here.
Although contributions via PR is an explicit way to help, we still call for any other ways.

- reply to other's issues if you could;
- help solve other user's problems;
- help review other's PR design;
- help review other's codes in PR;
- discuss about Kruise to make things clearer;
- advocate Kruise technology beyond GitHub;
- write blogs on Kruise and so on.

In a word, **ANY HELP IS CONTRIBUTION**.

## Join Kruise as a member

It is also welcomed to join Kruise team if you are willing to participate in Kruise community continuously and keep active.

### Requirements

- Have read the [Contributing to Kruise](./CONTRIBUTING.md) carefully
- Have read the [Contributor Covenant Code of Conduct](./CODE_OF_CONDUCT.md)
- Have submitted multi PRs to the community
- Be active in the community, may including but not limited
  - Submitting or commenting on issues
  - Contributing PRs to the community
  - Reviewing PRs in the community

### How to do it

You can do it in either of two ways:

- Submit a PR in the [openkruise/kruise](https://github.com/openkruise/kruise) repo
- Contact with the [community](./README.md#community) offline
