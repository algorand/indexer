## PostgresDB Vacuum

See [official documentation](https://www.postgresql.org/docs/current/routine-vacuuming.html#VACUUM-BASICS) for more details

Each update or delete query creates dead tuples. And running vacuum recovers disk space occupied by these tuples.
However, it creates a substantial amount of I/O traffic, which can cause poor performance for other active sessions. 

There are two types of vacuum: **autovacuum** or **vacuum** full.
In the default configuration, autovacuuming is enabled and the related configuration parameters are appropriately set.


### Autovacuum (Standard vacuum)

- can run in parallel with production database operations
- does not recover as much space full vacuum

### Vacuum full
- cannot be done in parallel with other use of the table;
- runs much more slowly

*Since vacuum full is much more costly,  the usual goal of routine vacuuming is to standar vacuum often enough to avoid full vacuum.*

### Vacuum configuration

Vacuum can be [configured](https://www.postgresql.org/docs/current/runtime-config-autovacuum.html) to be more or less aggressive

The vacuum threshold is defined as:

vacuum threshold = vacuum base threshold + vacuum scale factor * number of tuples

Insert threshold is defined as:

vacuum insert threshold = vacuum base insert threshold + vacuum insert scale factor * number of tuples
