# Demo environment

This environment demonstrates the different components required to test
and monitor an Indexer test scenario.

# To run
The following commands will refresh the environment. Indexer and generator are built from source as part of this process.
```
docker-compose rm && docker-compose build && docker-compose up
```

Once started the following services are available:
| service    | localhost port | login | password |
| ---------  | -------------- | ----- | -------- |
| indexer    | 8888           |       |          |
| generator  | 11111          |       |          |
| prometheus | 9090           |       |          |
| grafana    | 3000           | admin | admin    |
| postgres   | 5555           |       |          |

# Configure
To configure a different scenario modify the `docker-compose.yml` file. Change the `config.yml` file mounted for the generator service:
```
  generator:
    container_name: "algorand-block-generator"
    build:
      context: ./../../../
      dockerfile: ./cmd/block-generator/docker/Dockerfile-generator
    environment:
      PORT: "11111"
    ports:
      - 11111:11111
    volumes:
      # Override generator config file.
      - ../scenarios/config.mixed.jumbo.yml:/config/config.yml
```

# Dashboard
At this time we haven't installed the grafana dashboard automatically. When the services first start no metrics are available which seemed to cause import problems.

For now when you first start the environment the dashboard must be manually imported:
1. Connect to http://localhost:3000/dashboard/import
2. Login with admin/admin, then change the password (or press skip)
3. Click the blue `Upload JSON file` button.
4. Choose `grafana_indexer_dashboard.json` from this directory, click `Import`.

The dashboard will display some Indexer metrics when they become available.
