# release-train

<!--- everything between the next line and the "end action doc" comment is generated by script/generate --->
<!--- start action doc --->

hop on the release train

## Inputs

### check_pr_labels

default: `${{ github.event_name == 'pull_request' }}`

Instead of releasing, check that the PR has a label indicating the type of change.  Only literal 'true' will be treated as true.


### checkout_dir

default: `${{ github.workspace }}`

The directory where the repository is checked out.

### ref

default: `${{ github.ref }}`

The branch or tag to release.

### github_token

default: `${{ github.token }}`

The GitHub token to use for authentication. Must have `contents: write` permission if creating a release or tag.


### create_tag

Whether to create a tag for the release. Only literal 'true' will be treated as true.

### create_release

Whether to create a release. Only literal 'true' will be treated as true.

Implies `create_tag`.


### tag_prefix

default: `v`

The prefix to use for the tag. Defaults to `v`.


### initial_release_tag

default: `v0.0.0`

The tag to use if no previous version can be found.

Set to empty string to disable cause it to error if no previous version can be found.


### pre_release_hook

Command to run before creating the release. You may abort the release by exiting with a non-zero exit code.

Exit code 0 will continue the release. Exit code 10 will skip the release without error. Any other exit code will
abort the release with an error.

You may provide custom release notes by writing to the file at `$RELEASE_NOTES_FILE`:
```
  echo "my release notes" > "$RELEASE_NOTES_FILE"
```

You can update the git ref to be released by writing it to the file at `$RELEASE_TARGET`:
```
  # ... update some files ...
  git commit -am "prepare release $RELEASE_TAG"
  echo "$(git rev-parse HEAD)" > "$RELEASE_TARGET"
```

The environment variables `RELEASE_VERSION`, `RELEASE_TAG`, `PREVIOUS_VERSION`, `FIRST_RELEASE`, `GITHUB_TOKEN`,
`RELEASE_NOTES_FILE` and `RELEASE_TARGET` will be set.


### post_release_hook

Command to run after the release is complete. This is useful for adding artifacts to your release.

The environment variables `RELEASE_VERSION`, `RELEASE_TAG`, `PREVIOUS_VERSION`, `FIRST_RELEASE` and `GITHUB_TOKEN` 
will be set.


### validate_go_module

Validates that the name of the go module at the given path matches the major version of the release. For example,
validation will fail when releasing v3.0.0 when the module name is `my_go_module/v2`.

The path provided must be relative to inputs.checkout_dir.

Release train uses whatever version of go is in PATH. If you need to use a specific version of go, you can use
`WillAbides/setup-go-faster` to install it.


### no_release

If set to true, this will be a no-op. This is useful for creating a new repository or branch that isn't ready for
release yet.

Only literal 'true' will be treated as true.


## Outputs

### previous_ref

A git ref pointing to the previous release, or the current ref if no previous release can be found.


### previous_version

The previous version on the release branch.


### first_release

Whether this is the first release on the release branch. Either "true" or "false".


### release_version

The version of the new release. Empty if no release is called for.


### release_tag

The tag of the new release. Empty if no release is called for.


### change_level

The level of change in the release. Either "major", "minor", "patch" or "no change".


### created_tag

Whether a tag was created. Either "true" or "false".


### created_release

Whether a release was created. Either "true" or "false".
<!--- end action doc --->
