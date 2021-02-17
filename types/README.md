# Updating protocols.json

The main go-algorand repo contains a tool named `genconsensusconfig` that generates the supported consensus protocols version file.
The output of this tool can be used to generate the `protocol.json` in the following way :

```
~$ genconsensusconfig
~$ cat consensus.json | jq --sort-keys > protocols.json
~$ rm consensus.json
```

