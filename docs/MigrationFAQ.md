# Indexer 2.x to 3.x Migration FAQ

## Does Indexer 3.x change the REST API?

No. The Indexer 3.x REST API is fully backward compatible with Indexer 2.x. No change is required in applications or the Algorand SDK.

## Can the node used by Conduit be used to submit transactions?

No. The "follower" node used by Conduit is not a general purpose node and cannot submit transactions to the network. It has disabled many services compared to other types of nodes.

## What are the resource requirements for Conduit and Indexer 3.x compared to Indexer 2.x?

The storage requirements are much less. The Conduit follower node runs with non-archival storage requirements compared to the archival storage requirements needed by Indexer 2.x. For mainnet at round 32 million, this can save between 1.2TB and 1.6TB of storage.

The compute requirements are similar. With Conduit and a "follower" node performing a task previously handled by Indexer 2.x and an "archival" node.

The postgres requirements are the same for those reading the full history (existing indexer behavior). Using the new transaction pruning and filtering features in Conduit in some applications can save another 1.5 TB or more of storage.

## Are debian and rpm packages supported for Conduit and Indexer 3.x?

No, these are not currently supported.

## How long does it take to synchronize a new deployment?

With an optimal deployment, the process can take less than a week, but it is common for the process to take 3+ weeks. The process is typically disk and network bound.

For optimal performance ensure:
	* A fast network connection.
	* Low latency between Conduit, the follower node and postgres.
	* Fast disk, locally attached NVMe is ideal for Conduit and algod.
