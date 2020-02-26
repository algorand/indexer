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
