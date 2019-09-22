# Building
See travis.yml

# Testing
```
go clean -testcache; go test ./...
```

# Cleaning test cache
```
go clean -testcache
```

# Deploy to docker swarm
```
mkdir -p /var/tmp/blog-storage
mkdir -p /mnt/minio/data
docker stack deploy --compose-file /path/to/docker-compose.yml BLOGSTORAGESTACK
```

# Turn on transactions on mongo
## Option 1
1. Set command instruction on docker
2. run `docker/mongo/docker-entrypoint-initdb.d/rs0.js`

## Option 2 reconfig exists mongo instance
1. call for get current config
```
rs.config();
```
example output
```
config = {
    "_id" : "rs0",
    "version" : 1,
    "protocolVersion" : NumberLong(1),
    "writeConcernMajorityJournalDefault" : true,
    "members" : [
        {
            "_id" : 0,
            "host" : "mongo:27017",
            "arbiterOnly" : false,
            "buildIndexes" : true,
            "hidden" : false,
            "priority" : 1,
            "tags" : {

            },
            "slaveDelay" : NumberLong(0),
            "votes" : 1
        }
    ],
    "settings" : {
        "chainingAllowed" : true,
        "heartbeatIntervalMillis" : 2000,
        "heartbeatTimeoutSecs" : 10,
        "electionTimeoutMillis" : 10000,
        "catchUpTimeoutMillis" : -1,
        "catchUpTakeoverDelayMillis" : 30000,
        "getLastErrorModes" : {

        },
        "getLastErrorDefaults" : {
            "w" : 1,
            "wtimeout" : 0
        },
        "replicaSetId" : ObjectId("5d86f512301d68c373aa0be8")
    }
};
```
2. set `"host" : "mongo:27017"`

```
rs.reconfig(config, {force: true});
```
3. stop mongo and application
4. start mongo
5. start application