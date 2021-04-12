# `mule`

# Package Build Pipeline

- [Build Stages](#build-stages)
- [Examples](#examples)
- [FAQ](#faq)

# Build Stages

- package

    - Description:

        + Will build the following assets in `./tmp/node_pkgs/$OS_TYPE/$ARCH/$VERSION`:

            ```
            algorand-indexer_2.2.0-beta_amd64.deb
            algorand-indexer_2.2.0-beta_arm64.deb
            algorand-indexer_2.2.0-beta_armhf.deb
            algorand-indexer_darwin_amd64_2.2.0-beta.tar.bz2
            algorand-indexer_linux_amd64_2.2.0-beta.tar.bz2
            algorand-indexer_linux_arm_2.2.0-beta.tar.bz2
            algorand-indexer_linux_arm64_2.2.0-beta.tar.bz2
            ```

        + The version is gotten from `./.version`.

        > Note that if the branch doesn't already contain a tag that matches the version the operation will fail!

    - Operation(s):

            PKG_DIR = $(SRCPATH)/tmp/node_pkgs/$(OS_TYPE)/$(ARCH)/$(VERSION)
            rm -rf $(PKG_DIR)
            mkdir -p $(PKG_DIR)
            misc/release.py --outdir $(PKG_DIR)

- test

    - Description:

        + Runs all go tests.

    - Operation(s):

            go test ./...

- test-package

    - Description:

        + Creates a test database in Postgres and runs the end-to-end tests.

    - Operation(s):

            mule/e2e.sh

- sign

    - Description:

        + The `gpg-agent` should be seeded with the signing key's passphrase, and the socket is then mounted
          to the docker container.

        + All the `deb` and `tar.bz2` build artifacts created in the `package` step are then signed.

        + In addition, `md5` and `sha` hashes are calculated and captured which are then used to build the
          [releases page](https://releases.algorand.com/). These are also signed.

    - Operation(s):

            mule/sign.sh

- stage

    - Description:

        + Uploads all build assets and their detached signatures to S3.

    - Operation(s):

        + `mule` will internally make the following call using the `boto3` library:

            ```
            aws s3 cp $HOME/projects/indexer/tmp/node_pkgs/linux/amd64/$VERSION s3://$STAGING/indexer/$VERSION
            ```

- deploy

    - Description:

        + Uses the `aptly` tool.

        + Adds the new amd64 deb package to the `indexer` local repository, creates a new snapshot of the repo
          and then pushes the snapshot to the remote mirror on S3.

    - Operation(s):

            mule/deploy.sh

# Examples

### Packaging

    mule -f mule.yaml package

### Testing

    mule -f mule.yaml test
    mule -f mule.yaml test-package

### Signing

    mule -f mule.yaml sign

### Staging

    STAGING=the-staging-area VERSION=2.2.0-beta mule -f mule.yaml stage

### Deploying

    mule -f mule.yaml deploy-rpm

# FAQ

Q. Does `mule` ensure that environment variables are set?

A. It depends on the task, but generally, yes.

Most `mule` tasks call targets that invoke a shell script, so those will check for needed env vars. Others, like the one in question here,
call an internal library function to upload to S3 using `boto3`, so that will expect the env var to be set in the calling environment.

For example, here is an example of setting environment variables when uploading to staging:

```
STAGING=the-staging-area VERSION=2.2.0-beta mule -f mule.yaml stage
```

