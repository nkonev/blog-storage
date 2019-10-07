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


Migration
```js
db.getCollectionNames().forEach(function(collname) {
    if(collname.startsWith('user') && collname != 'userData'){
        db[collname].find().forEach( function(userDoc) {
            db.global_objects.updateOne({_id: userDoc._id}, { $set: { filename: userDoc.filename, published: userDoc.published } })
        } );
    }
})
```