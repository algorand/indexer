# Filewriter Exporter

Write the block data to a file.

Data is written to one file per block in JSON format.

# Config
```yaml
exporter:
  - name: file_writer
    config:
      - block-dir: "path to write block data"
        # override the filename pattern.
        filename-pattern: "%[1]d_block.json"
        # exclude the vote certificate from the file.
        drop-certificate: false
```

