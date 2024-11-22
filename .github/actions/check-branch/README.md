# Check Branch

This action gets the head ref or source branch of the pull request in a workflow to run a checks on the branch name. The check verify that the branch name starts with a placeholder `types` and followed by another placeholder `separators`.

The input `types` default values are:

- major
- release
- minor
- feature
- feat
- patch
- issue
- hotfix
- dependabot
- whitesource/

The input `separators` default value are `-` and `\`.

## Usage

See [action.yml](action.yml).

## Examples

```yaml
---
name: check branch name

on:
  pull_request:

jobs:
  bump:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2

      - name: Check branch
        uses: ./.github/actions/check-branch/
```

## How does it work

This simple action runs a [check-branch script](check-branch.sh)
