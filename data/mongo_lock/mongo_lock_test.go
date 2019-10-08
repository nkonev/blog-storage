package mongo_lock

import (
	"context"
	"fmt"
	"github.com/nkonev/blog-storage/utils"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	setup()
	retCode := m.Run()
	shutdown()
	os.Exit(retCode)
}

func shutdown() {

}

func setup() {
	s, _, _ := utils.InitFlag("../../config-dev/config.yml")
	utils.InitViper(s)

	log.Info("Set up")
	utils.DropMongo()
}

func TestHangsOnLocked(t *testing.T) {
	mongoClient := utils.GetMongoClient()
	defer mongoClient.Disconnect(context.TODO())

	testLock := "test_locked_1"
	coll := mongoClient.Database(utils.GetMongoDbName(utils.GetMongoUrl())).Collection(testLock)
	result, err := coll.InsertOne(context.TODO(), GetIdDoc())
	if err != nil {
		log.Fatalf("error during insert: %v", err)
	}
	log.Infof("InsertId %v", result.InsertedID)

	lock := NewMongoLock(mongoClient, testLock)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		lock.AcquireLock()
	}()
	go func() {
		defer wg.Done()
		time.Sleep(time.Second * 4)
		coll.DeleteOne(context.TODO(), GetIdDoc())
	}()

	wg.Wait()
}

func TestLockIsValid(t *testing.T) {
	mongoClient := utils.GetMongoClient()
	defer mongoClient.Disconnect(context.TODO())

	testLock := "test_locked_2"

	lock := NewMongoLock(mongoClient, testLock)

	var wg sync.WaitGroup
	wg.Add(2)

	counter := 0

	go func() {
		defer wg.Done()
		lock.AcquireLock()
		for i := 0; i < 1000; i++ {
			counter++
		}
		lock.ReleaseLock()
	}()
	go func() {
		defer wg.Done()
		lock.AcquireLock()
		for i := 0; i < 1000; i++ {
			counter++
		}
		lock.ReleaseLock()
	}()
	wg.Wait()

	assert.Equal(t, 2000, counter)

}

func TestLockIsValidForHighConcurrentEnvironment(t *testing.T) {
	mongoClient := utils.GetMongoClient()
	defer mongoClient.Disconnect(context.TODO())

	testLock := "test_locked_2"

	lock := NewMongoLock(mongoClient, testLock)

	instances := 1000
	var wg sync.WaitGroup
	wg.Add(instances)

	counter := 0

	for i := 0; i < instances; i++ {
		go func() {
			defer wg.Done()
			lock.AcquireLock()
			counter++
			lock.ReleaseLock()
		}()
	}

	wg.Wait()

	assert.Equal(t, instances, counter)
}

func hasIndex(coll *mongo.Collection, indexName string) bool {
	cursor, e := coll.Indexes().List(context.TODO())
	defer cursor.Close(context.TODO())
	if e != nil {
		log.Fatalf("error during listing indexes: %v", e)
	}
	var hasUniqueIndex bool
	for cursor.Next(context.TODO()) {
		var doc bson.D
		e := cursor.Decode(&doc)
		str := fmt.Sprintf("%v", doc)
		if strings.Contains(str, indexName) {
			hasUniqueIndex = true
			break
		}
		log.Infof("Indexes %v %v", str, e)
	}
	return hasUniqueIndex
}

func TestLockIsReleased(t *testing.T) {
	mongoClient := utils.GetMongoClient()
	defer mongoClient.Disconnect(context.TODO())

	testLock := "test_locked_3"
	coll := mongoClient.Database(utils.GetMongoDbName(utils.GetMongoUrl())).Collection(testLock)

	lock := NewMongoLock(mongoClient, testLock)

	assert.False(t, hasIndex(coll, "unique_id"))

	lock.AcquireLock()

	assert.True(t, hasIndex(coll, "unique_id"))

	one := coll.FindOne(context.TODO(), GetIdDoc())
	raws, err := one.DecodeBytes()
	if err != nil {
		log.Fatalf("error during find: %v", err)
	}
	assert.NotNil(t, raws)

	lock.ReleaseLock()

	oneAfter := coll.FindOne(context.TODO(), GetIdDoc())
	rawsAfter, errAfter := oneAfter.DecodeBytes()
	assert.NotNil(t, errAfter)
	assert.Nil(t, rawsAfter)
}
