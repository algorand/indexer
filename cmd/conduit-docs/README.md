## conduit-docs

`conduit-docts` is a utility used to generate markdown files from Conduit plugin config structs.  
It's usage is designed around `//go:generate`, which should be run on each plugin's `*.go` config file.

### File Names
The output file name will be the go package name suffixed with `.md`

To override the output filename, use a comment beginning with `//Name: ` The remaining portion of the comment will be
suffixed with `.md` and used as the file name.

### Converting structs to graphs
The structs within the file will be turned into a graph listing the yaml key, the type of the entry, and
using the comment above each field as a description.

For example,

```go
package example 

//go:generate conduit-docs output-dir

type Config struct {
	// description field of the graph
	Field string `yaml:"field-name"`
}
```
will output the following graph:
<table>
<tr>
<th>key</th><th>type</th><th>description</th>
</tr>
<tr>
<td>field-name</td><td>string</td><td>description field of the graph</td>
</tr>
</table>

### Additional data in header/footer

In addition to converting structs to documentation graphs, you can add additional markdown to the top and bottom of the
output document via comments denoted with certain values.

A multi-line comment beginning with `/*Header` will be added to the beginning of the document.

And a multi-line comment beginning with `/*Footer` will be added to the end of the document.

So the following config,

```go
package example 

//go:generate conduit-docs output-dir

/*Header
## Config Documentation
This is a config which is being documented.
 */

type Config struct {
	// description field of the graph
	Field string `yaml:"field-name"`
}
/*Footer
## Examples
These are some examples  
`Foo=bar`
 */
```
will output the following document:
## Config Documentation
This is a config which is being documented.
<table>
<tr>
<th>key</th><th>type</th><th>description</th>
</tr>
<tr>
<td>field-name</td><td>string</td><td>description field of the graph</td>
</tr>
</table>

## Examples
These are some examples  
`Foo=bar`