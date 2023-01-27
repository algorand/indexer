package blockprocessor

//go:generate conduit-docs ../../../../conduit-docs/

//Name: conduit_processors_blockevaluator

/*Header
## The <code>block_processor</code> Plugin
This plugin runs a local ledger, processing blocks passed to it, and adding

*/

// Config configuration for a block processor
type Config struct {
	/* <code>catchpoint</code> to initialize the local ledger to.<br/>
	For more data on ledger catchpoints, see the
	<a hre=https://developer.algorand.org/docs/run-a-node/operations/catchup/>Algorand developer docs</a>
	*/
	Catchpoint string `yaml:"catchpoint"`

	// <code>ledger-dir</code> is the directory which contains the ledger.
	LedgerDir string `yaml:"ledger-dir"`
	// <code>algod-data-dir</code> is the algod data directory.
	AlgodDataDir string `yaml:"algod-data-dir"`
	// <code>algod-token</code> is the API token for Algod usage.
	AlgodToken string `yaml:"algod-token"`
	// <code>algod-addr</code> is the address of the Algod server
	AlgodAddr string `yaml:"algod-addr"`
}

/*Footer

### Example Configs


```yaml
config:
  - any:
    - tag: ""
	  expression: ""
	  expression-type: ""
```
*/
