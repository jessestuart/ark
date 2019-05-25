# Upgrading to Velero 1.0

## Prerequisites
- Velero v0.11 installed. If you're not already on v0.11, see the [instructions for upgrading to v0.11][0]. **Upgrading directly from v0.10.x or earlier to v1.0 is not supported!**
- (Optional, but strongly recommended) Create a full copy of the object storage bucket(s) Velero is using. Part 1 of the upgrade procedure will modify the contents of the bucket, so we recommend creating a backup copy of it prior to upgrading.

## Instructions

### Part 1 - Rewrite Legacy Metadata

#### Overview

You need to replace legacy metadata in object storage with updated versions **for any backups that were originally taken with a version prior to v0.11 (i.e. when the project was named Ark)**. While Velero v0.11 is backwards-compatible with these legacy files, Velero v1.0 is not.

_If you're sure that you do not have any backups that were originally created prior to v0.11 (with Ark), you can proceed directly to Part 2._

We've added a CLI command to [Velero v0.11.1][1], `velero migrate-backups`, to help you with this. This command will:

- Replace `ark-backup.json` files in object storage with equivalent `velero-backup.json` files. 
- Create `<backup-name>-volumesnapshots.json.gz` files in object storage if they don't already exist, containing snapshot metadata populated from the backups' `status.volumeBackups` field*. 

_*backups created prior to v0.10 stored snapshot metadata in the `status.volumeBackups` field, but it has subsequently been replaced with the `<backup-name>-volumesnapshots.json.gz` file._


#### Instructions
1. Download the [v0.11.1 release tarball][1] tarball for your client platform.

1. Extract the tarball:
    ```bash
    tar -xvf <RELEASE-TARBALL-NAME>.tar.gz -C /dir/to/extract/to 
    ```

1. Move the `velero` binary from the Velero directory to somewhere in your PATH.

1. Scale down your existing Velero deployment:
    ```bash
    kubectl -n velero scale deployment/velero --replicas 0
    ```

1. List all of your backup storage locations:
    ```bash
    velero backup-location get
    ```

1. For each backup storage location that you want to use with Velero 1.0, replace any legacy pre-v0.11 backup metadata with the equivalent current formats:
    ```bash
    # - BACKUP_LOCATION_NAME is the name of a backup location from the previous step, whose
    #   backup metadata will be updated in object storage
    # - SNAPSHOT_LOCATION_NAME is the name of the volume snapshot location that Velero should
    #   record volume snapshots as existing in (this is only relevant if you have backups that
    #   were originally taken with a pre-v0.10 Velero/Ark.)
    velero migrate-backups \
        --backup-location <BACKUP_LOCATION_NAME> \
        --snapshot-location <SNAPSHOT_LOCATION_NAME>
    ```

1. Scale up your deployment:
    ```bash
    kubectl -n velero scale deployment/velero --replicas 1
    ```

### Part 2 - Upgrade Components to Velero 1.0

#### Overview

#### Instructions

1. Download the [v1.0 release tarball][2] tarball for your client platform.

1. Extract the tarball:
    ```bash
    tar -xvf <RELEASE-TARBALL-NAME>.tar.gz -C /dir/to/extract/to 
    ```

1. Move the `velero` binary from the Velero directory to somewhere in your PATH, replacing any existing pre-1.0 `velero` binaries.

1. Update the image for the Velero deployment and daemon set (if applicable):
    ```bash
    kubectl -n velero set image deployment/velero velero=gcr.io/heptio-images/velero:v1.0.0
    kubectl -n velero set image daemonset/restic  restic=gcr.io/heptio-images/velero:v1.0.0
    ```

[0]: https://heptio.github.io/velero/v0.11.0/migrating-to-velero
[1]: https://github.com/heptio/velero/releases/tag/v0.11.1
[2]: https://github.com/heptio/velero/releases/tag/v1.0.0
