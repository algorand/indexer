# Filewriter Exporter

Write the block data to a file.

Data is written to one file per block in JSON format.

By default data is written to the filewriter plugin directory inside the indexer data directory.

# Config
```yaml
exporter:
  - name: file_writer
    config:
      - block-dir: "override default block data location."
        # override the filename pattern.
        filename-pattern: "%[1]d_block.json"
        # exclude the vote certificate from the file.
        drop-certificate: false
```

