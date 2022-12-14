## Writing Blocks to Files Using Conduit

This guide will take you step by step through a specific application of some
Conduit plugins. We will detail each of the steps necessary to solve our problem, and point out documentation and tools
useful for both building and debugging conduit pipelines.

## Our Problem Statement

For this example, our task is to ingest blocks from an Algorand network (we'll use Testnet for this),
and write blocks to files. 

Additionally, we don't care to see data about transactions which aren't sent to us, so we'll filter out all transactions
which are not sending either algos or some other asset to our account.

## Getting Started

First we need to make sure we have Conduit installed. Head over to [GettingStarted.md](./GettingStarted.md)
in order to get more details on how to install Conduit. We'll just build it from source:  
```bash
git clone https://github.com/algorand/indexer.git $HOME/indexer
cd indexer
make conduit
alias conduit=$HOME/indexer/cmd/conduit/conduit
```

Now that we have Conduit installed we can take a look at the options for supported plugins with 
```
conduit list
```
The current list ends up being 
```
importers:
  algod       - Importer for fetching blocks from an algod REST API.
  file_reader - Importer for fetching blocks from files in a directory created by the 'file_writer' plugin.

processors:
  block_evaluator  - Local Ledger Block Processor
  filter_processor - FilterProcessor Filter Processor
  noop             - noop processor

exporters:
  file_writer - Exporter for writing data to a file.
  noop        - noop exporter
  postgresql  - Exporter for writing data to a postgresql instance.
```

For our conduit pipeline we're going to use the `algod` importer, a `filter_processor`, and of course the
`file_writer` exporter.  
To get more details about each of these individually, and the configuration variables required and available for them, 
we can again use the list command. For example, 
```
conduit list exporters file_writer
```
Returns the following:
```
name: "file_writer"
config:
  # BlocksDir is an optional path to a directory where block data will be
  # stored. The directory is created if it doesn't exist. If not present the
  # plugin data directory is used.
  block-dir: "/path/to/block/files"
  # FilenamePattern is the format used to write block files. It uses go
  # string formatting and should accept one number for the round.
  # If the file has a '.gz' extension, blocks will be gzipped.
  # Default: "%[1]d_block.json"
  filename-pattern: "%[1]d_block.json"
  # DropCertificate is used to remove the vote certificate from the block data before writing files.
  drop-certificate: true
```

## Setting Up Our Pipeline

Let's start assembling a configuration file which describes our conduit pipeline. For that we'll run 
```
conduit init
```
This will create a configuration directory if we don't provide one to it, and write a skeleton config file
there which we will use as the starting point for our pipeline. Here is the config file which the `init` subcommand has
written for us:
```yaml
# Generated conduit configuration file.
log-level: INFO
# When enabled prometheus metrics are available on '/metrics'
metrics:
  mode: OFF
  addr: ":9999"
  prefix: "conduit"
# The importer is typically an algod archival instance.
importer:
  name: algod
  config:
    netaddr: "your algod address here"
    token: "your algod token here"
# One or more processors may be defined to manipulate what data
# reaches the exporter.
processors:
# An exporter is defined to do something with the data.
# Here the filewriter is defined which writes the raw block
# data to files.
exporter:
  name: file_writer
  config:
  # optionally provide a different directory to store blocks.
  #block-dir: "path where data should be written"
```
## Setting up our Importer
We can see the specific set of plugins defined for our pipeline--an `algod` importer and `file_writer` exporter.
Now we will fill in the proper fields for these. I've got a local instance of algod running at `127.0.0.1:8080`,
with an API token of `e36c01fc77e490f23e61899c0c22c6390d0fff1443af2c95d056dc5ce4e61302`. If you need help setting up
algod, you can take a look at the [go-algorand docs](https://github.com/algorand/go-algorand#getting-started) or our
[developer portal](https://developer.algorand.org/).

Here is the completed importer config:
```yaml
importer:
    name: algod
    config:
      netaddr: "http://127.0.0.1:8080"
      token: "e36c01fc77e490f23e61899c0c22c6390d0fff1443af2c95d056dc5ce4e61302"
```

## Setting up our Processor

The processor section in our generated config is empty, so we'll need to fill in that section with the proper data
for the filter processor. We can paste in the output of our list command for that.
```bash
> conduit list processors filter_processor
name: filter_processor
config:
  # Filters is a list of boolean expressions that can search the payset transactions.
  # See README for more info.
  filters:
    - any:
      - tag: txn.rcv
        expression-type: exact
        expression: "ADDRESS"
```
The filter processor uses the tag of a property and allows us to specify an exact value to match or a regex.
For our use case we'll grab the address of a wallet I've created on testnet, `NVCAFYNKJL2NGAIZHWLIKI6HGMTLYXL7BXPBO7NXX4A7GMMWKNFKFKDKP4`.

That should give us exactly what we want, a filter that only allows transaction through for which the receiver is my
account. However, there is a lot more you can do with the filter processor. To learn more about the possible uses, take
a look at the individual plugin documentation [here](../plugins/filter_processor.md).

## Setting up our Exporter

For the exporter the setup is simple. No configuration is necessary because it defaults to a directory inside the
conduit data directory. In this example I've chosen to override the default and set the directory output of my blocks
to a temporary directory, `block-dir: "/tmp/conduit-blocks/"`. 

## Running the pipeline
Now we should have a fully valid config, so let's try it out. Here's the full config I ended up with
(with comments removed)
```yaml
log-level: "INFO"
importer:
    name: algod
    config:
      netaddr: "http://127.0.0.1:8080"
      token: "e36c01fc77e490f23e61899c0c22c6390d0fff1443af2c95d056dc5ce4e61302"
processors:
  name: filter_processor
  config:
    filters:
      - any:
       - tag: txn.rcv
         expression-type: exact
         expression: "NVCAFYNKJL2NGAIZHWLIKI6HGMTLYXL7BXPBO7NXX4A7GMMWKNFKFKDKP4"
exporter:
    name: file_writer
    config:
      block-dir: "/tmp/conduit-blocks/"
```

There are two things to address before our example becomes useful.
1. We need to get a payment transaction to our account.

For me, it's easiest to use the testnet dispenser, so I've done that. You can look at my transaction for yourself,
block #26141781 on testnet.
2. Skip rounds

To avoid having to run algod all the way from genesis to the most recent round, you can use catchpoint catchup to
fast-forward to a more recent block. Similarly, we want to be able to run Conduit pipelines from whichever round is
most relevant and useful for us.
To run conduit from a round other than 0, use the `--next-round-override` or `-r` flag. 

Now let's run the command!
```bash
> conduit -d /tmp/conduit-tmp/ --next-round-override 26141781
```

Once we've processed round 26141781, we should see our transaction show up!

```bash
> cat /tmp/conduit-blocks/* | grep payset -A 14
  "payset": [
    {
      "hgi": true,
      "sig": "DI4oMkUT01LAs5XT55qcZ3VCY8Wn2WrAZpntzFu2bTz9xnzaObmp5TOTUF5/PVVFCn14hXKyF3/LTZTUJylaDw==",
      "txn": {
        "amt": 10000000,
        "fee": 1000,
        "fv": 26141780,
        "lv": 26142780,
        "rcv": "NVCAFYNKJL2NGAIZHWLIKI6HGMTLYXL7BXPBO7NXX4A7GMMWKNFKFKDKP4",
        "snd": "GD64YIY3TWGDMCNPP553DZPPR6LDUSFQOIJVFDPPXWEG3FVOJCCDBBHU5A",
        "type": "pay"
      }
    }
  ]
```

There are many other existing plugins and use cases for Conduit! Take a look through the documentation and don't
hesitate to open an issue if you have a question. If you want to get a deep dive into the different types of filters
you can construct using the filter processor, take a look at our [filter guide](./FilterDeepDive.md).