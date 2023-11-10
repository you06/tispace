# TiSpace

TiSpace is a tool to estimate the space usage in TiDB in 2 ways.

* Memory usage for large transaction.
* Disk usage for table.

## Usage

```bash
» ./tispace -schema tests/sysbench.sql -rows=10000000 -sample=10000
sample 1000 lines cost 9.402334ms
insert 10000000 rows with memory cost: 2.33GB(2500000000 bytes)
```
