
# [Go modules](https://github.com/golang/go/wiki/Modules#quick-start)

## Init go modules
```
go mod init
```

## Download go modules
```
go build
```

```
curl -v -F 'file=@./utils/utils.go' http://127.0.0.1:1234/upload
```

```bash
go get github.com/vektra/mockery/.../
```
Caution! Don' t paste it to IntelliJ terminal - there is GO111MODULE=on.

(Check `echo $GO111MODULE`)


```
(cd client/; mockery -name RestClient)
```

# Building
```
go test ./...
(cd frontend; npm i; npm run prod;)
go get github.com/gobuffalo/packr/v2/packr2@v2.0.1
packr2 build
```

# Deploy to docker swarm
```
mkdir -p /var/tmp/blog-store
mkdir -p /mnt/minio/data
docker stack deploy --compose-file /path/to/docker-compose.yml BLOGSTORESTACK
```