
### SubConfig
<table>
<tr>
<th>key</th><th>type</th><th>description</th>

<tr><td>tag</td><td>string</td><td> <code>tag</code> is the tag of the struct to analyze.<br/>
	It can be of the form `txn.*` where the specific ending is determined by the field you wish to filter on.<br/>
	It can also be a field in the ApplyData.
</td></tr>

<tr><td>expression-type</td><td>expression.Type</td><td> <code>expression-type</code> is the type of comparison applied between the field, identified by the tag, and the expression.<br/>
	<ul>
		<li>exact</li>
		<li>regex</li>
		<li>less-than</li>
		<li>less-than-equal</li>
		<li>greater-than</li>
		<li>great-than-equal</li>
		<li>equal</li>
		<li>not-equal</li>
	</ul>
</td></tr>

<tr><td>expression</td><td>string</td><td><code>expression</code> is the user-supplied part of the search or comparison.
</td></tr>
</table>


### Config
<table>
<tr>
<th>key</th><th>type</th><th>description</th>

<tr><td>search-inner</td><td>bool</td><td><code>search-inner</code> configures the filter processor to recursively search inner transactions for expressions.
</td></tr>

<tr><td>omit-group-transactions</td><td>bool</td><td><code>omit-group-transactions</code> configures the filter processor to return the matched transaction without its grouped transactions.
</td></tr>

<tr><td>filters</td><td>[]map[string][]SubConfig</td><td> <code>filters</code> are a list of SubConfig objects with an operation acting as the string key in the map

	filters:
		- [any,all,none]:
			expression: ""
			expression-type: ""
			tag: ""
</td></tr>
</table>

