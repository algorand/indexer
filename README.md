# indexer
searchable history and current state

# Bootstrapping Development 

### Setup Private Network
Start a private network
```
~$ goal network create -n indexer-network -r ~/private-network -d ~/private-network/data -k ~/private-network/data/kmd -t /path/to/go-algorand/test/testdata/nettemplates/TwoNodes50EachFuture.json
~$ goal network start ~/private-network/
~$ goal node status -d ~/private-network/Primary
```

Start pingpong to generate data for indexer.
```
~$ pingpong run -d private-network/Primary/
```

### Start with dummy DB
```
~$ ./indexer daemon -d ~/algorand/private-network/Primary/ --genesis ~/algorand/private-network/Primary/genesis.json  -n
```

### Start with postgres
Start postgres docker container
```
~$ docker pull postgres
~$ docker run --rm   --name pg-docker -e POSTGRES_PASSWORD=docker -d -p 5432:5432 -v $HOME/docker/volumes/postgres:/var/lib/postgresql/data  postgres
```

You can test postgres with an SQL client like SQuirreL, the JDBC connection string to the default `postgres` database is `jdbc:postgresql://localhost:5432/postgres`

Start indexer
```
~$ ./indexer daemon -d ~/algorand/private-network/Primary/ --genesis ~/algorand/private-network/Primary/genesis.json --postgres "host=localhost port=5432 user=postgres password=docker dbname=postgres sslmode=disable"
```

# Code Generation

### oapi-codegen
The gontents of **api/models.go** and **api/routes.go** are generated with the following:
```
oapi-codegen -package generated -type-mappings integer=uint64 -generate types -o ../oapi-codegen/chi/types.go indexer.oas3.yml
oapi-codegen -package generated -type-mappings integer=uint64 -generate server -o ../oapi-codegen/chi/route.go indexer.oas3.yml
```

### openapi-generator
**This didn't generate input validators. Remove this section once the final validator is chosen.**
The contents of **api/gen** was made with opeanapi-generator-cli, specifically:
```
java -jar openapi-generator-cli.jar generate -i merged.oas3.yml -g go-server --type-mappings=integer=uint64 --additional-properties=sourceFolder=gen --additional-properties=packageName=api -o /path/to/indexer/api/
```

A number of files are ignored according to the definition in **api/.openapi-generator-ignore**
