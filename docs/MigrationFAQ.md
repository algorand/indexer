# Indexer 2.x to 3.x Migration FAQ

## Does Indexer 3.x change the REST API?

No. The Indexer 3.x REST API is fully backward compatible with Indexer 2.x. No change is required in applications or the Algorand SDK.

## Can the node used by Conduit be used to submit transactions?

No. The "follower" node used by Conduit is not a general purpose node and cannot submit transactions to the network. It has disabled many services compared to other types of nodes.

## Can I re-use a 2.x database?

Yes. The schema for 2.15.4 and 3.2.0 are fully compatible. If you have an existing database, or a database backup, it can be used to speed up the setup time.

If you provide Conduit with the follower node's admin API token, then Conduit will attempt to run fast catchup automatically by looking at the latest round in the database and finding an appropriate catchpoint. Once the catchpoint is restored, it will resume with normal block processing.

## How long does it take to synchronize a new deployment?

With an optimal deployment, the process can take less than a week, but it is common for the process to take 3+ weeks. The process is typically disk and network bound.

For optimal performance ensure:
* A fast network connection.
* Low latency between Conduit, the follower node and postgres.
* Fast disk, locally attached NVMe is ideal for Conduit and algod.

## What are the resource requirements for Conduit and Indexer 3.x compared to Indexer 2.x?

The storage requirements are much less. The Conduit follower node runs with non-archival storage requirements compared to the archival storage requirements needed by Indexer 2.x. For mainnet at round 32 million, this can save between 1.2TB and 1.6TB of storage.

The compute requirements are similar. With Conduit and a "follower" node performing a task previously handled by Indexer 2.x and an "archival" node.

The postgres requirements are the same for those reading the full history (existing indexer behavior). Using the new transaction pruning and filtering features in Conduit in some applications can save another 1.5 TB or more of storage.

## Are debian and rpm packages supported for Conduit and Indexer 3.x?

No, these are not currently supported.


## Conduit error: "operation not available during catchup"

You will see this error if the node is running fast catchup while Conduit is being started. The process will terminate after the maximum number of retries.

Some operators are used to manually running fast catchup as part of their deployment process. With Conduit, running fast catchup manually is no longer required. By setting the node's admin API token in the Conduit config, fast catchup will run automatically if necessary.

Docker users should avoid the `FAST_CATCHUP=1` environment setting.

If you see this message for a new deployment, recreate the follower node and ensure fast catchup is not used.

## Conduit error: "initializing block round _____ but next round to account is 0"

You will see this error if the node is running fast catchup while Conduit is being started. The process will terminate after the maximum number of retries.

This can also occur during initialization you reset the database or use the start-at-round option.

If this happens during a new installation, it is best to reset everything and start from the beginning. If it happens for a long running deployment you should contact Algorand Inc for support.

## Conduit error: "failed to set sync round on the ledger"

You will see this error if the node is not configured as a "follower" node. The process will terminate after the maximum number of retries.

Please refer to the [Using Conduit to Populate an Indexer Database](https://github.com/algorand/conduit/blob/master/docs/tutorials/IndexerWriter.md) documentation.

## Help! My question is not answered here!

If your question is not answered here [please join us on discord](https://discord.com/invite/algorand). We routinely monitor the `conduit` channel along with many Conduit operators.
