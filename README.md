# TiSpace

TiSpace is a tool to estimate the space usage in TiDB in 2 ways.

* Memory usage for large transaction.
* Disk usage for table.

## Usage Examples

```bash
» ./tispace -rows=100000000 -sample=10000 -schema tests/sysbench.sql -drop-value=false -mode=insert
sample 10000 lines cost 62.134416ms, 6.213µs per row
insert 100000000 rows with memory cost: 23.38GB(25100000000 bytes)

» ./tispace -rows=100000000 -sample=10000 -schema tests/sysbench.sql -drop-value=false -mode=update
sample 10000 lines cost 50.446875ms, 5.044µs per row
update 100000000 rows with memory cost: 26.92GB(28900000000 bytes)

» ./tispace -rows=100000000 -sample=10000 -schema tests/sysbench.sql -drop-value=false -mode=delete
sample 10000 lines cost 25.925083ms, 2.592µs per row
delete 100000000 rows with memory cost: 5.22GB(5600000000 bytes)

» ./tispace -rows=100000000 -sample=10000 -schema tests/sysbench.sql -drop-value=true -mode=insert
sample 10000 lines cost 62.692875ms, 6.269µs per row
drop 20000 keys cost 4.136875ms
insert 100000000 rows with memory cost: 5.22GB(5600000000 bytes)

» ./tispace -rows=100000000 -sample=10000 -schema tests/sysbench.sql -drop-value=true -mode=update
sample 10000 lines cost 41.03ms, 4.103µs per row
drop 30000 keys cost 6.6055ms
update 100000000 rows with memory cost: 8.66GB(9300000000 bytes)

» ./tispace -rows=100000000 -sample=10000 -schema tests/sysbench.sql -drop-value=true -mode=delete
sample 10000 lines cost 29.423041ms, 2.942µs per row
drop 20000 keys cost 3.471666ms
delete 100000000 rows with memory cost: 5.22GB(5600000000 bytes)
```
