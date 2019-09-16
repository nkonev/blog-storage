package repository

import (
	"context"
	"errors"
	"github.com/nkonev/blog-storage/logger"
	. "github.com/nkonev/blog-storage/logger"
	"github.com/nkonev/blog-storage/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const Id = "_id"
const filename = "filename"
const published = "published"

const collectionGlobalObjects = "global_objects"

// https://vkt.sh/go-mongodb-driver-cookbook/
type UserFileDto struct {
	Id        primitive.ObjectID `bson:"_id"` // mongo document id equal to minio object jd
	Filename  string
	Published bool
}

type GlobalIdDoc struct {
	UserId int64
}

type UserFileRepository struct {
	mongo *mongo.Client
}

func NewUserFileRepository(mongo *mongo.Client) *UserFileRepository {
	return &UserFileRepository{mongo: mongo}
}

type GlogalIdRepository struct {
	mongo *mongo.Client
}

func NewGlogalIdRepository(mongo *mongo.Client) *GlogalIdRepository {
	return &GlogalIdRepository{mongo: mongo}
}

func NewGlogalIdDoc(userId int) *GlobalIdDoc {
	return &GlobalIdDoc{UserId: int64(userId)}
}

func ToFileMongoDto(c *mongo.Cursor) (*UserFileDto, error) {
	var elem UserFileDto
	err := c.Decode(&elem)
	if err != nil {
		return nil, err
	}
	return &elem, nil
}

func GetIdDoc(objectId string) (*bson.D, error) {
	ids, e := primitive.ObjectIDFromHex(objectId)
	if e != nil {
		return nil, e
	}
	ds := bson.D{{Id, ids}}
	return &ds, nil
}

func GetUpdateDoc(p bson.M) bson.M {
	update := bson.M{"$set": p}
	return update
}

func (r *GlogalIdRepository) GetNextGlobalId(userIdV int) (*string, error) {
	database := utils.GetMongoDatabase(r.mongo)
	globalIdDoc := NewGlogalIdDoc(userIdV)
	result, e := database.Collection(collectionGlobalObjects).InsertOne(context.TODO(), globalIdDoc)
	if e != nil {
		return nil, e
	}
	idMongo := result.InsertedID.(primitive.ObjectID).Hex()
	return &idMongo, nil
}

func (r *GlogalIdRepository) GetUserIdByGlobalId(objectId string) (int, error) {
	ids, e := primitive.ObjectIDFromHex(objectId)
	if e != nil {
		return 0, e
	}
	database := utils.GetMongoDatabase(r.mongo)

	ms := bson.M{Id: ids}
	one := database.Collection(collectionGlobalObjects).FindOne(context.TODO(), ms)
	if one.Err() != nil {
		if one.Err() != mongo.ErrNoDocuments {
			logger.Logger.Errorf("Error during get user id by global id %v", objectId)
		} else {
			logger.Logger.Infof("No documents found by global id %v", objectId)
		}
		return 0, one.Err()
	}
	var elem GlobalIdDoc
	if err := one.Decode(&elem); err != nil {
		return 0, err
	}
	return int(elem.UserId), nil
}

func (r *UserFileRepository) GetMetainfoFromMongo(objectId string, userBucketName string) (*UserFileDto, error) {
	database := utils.GetMongoDatabase(r.mongo)
	var userFilesCollection *mongo.Collection = database.Collection(userBucketName)

	ds, err := GetIdDoc(objectId)
	if err != nil {
		logger.Logger.Errorf("Error during creating id document %v", objectId)
		return nil, err
	}

	one := userFilesCollection.FindOne(context.TODO(), ds)
	if one == nil {
		return nil, errors.New("Unexpected nil by id " + objectId)
	}
	if one.Err() != nil {
		logger.Logger.Errorf("Error during querying record from mongo by key %v", objectId)
		return nil, one.Err()
	}

	var elem UserFileDto
	if err := one.Decode(&elem); err != nil {
		if err == mongo.ErrNoDocuments {
			logger.Logger.Errorf("No documents found by key %v", objectId)
		}
		return nil, err
	}
	return &elem, nil
}

func (r *UserFileRepository) RenameUserFile(objId string, newname string, userBucketName string) error {
	database := utils.GetMongoDatabase(r.mongo)
	var userFilesCollection *mongo.Collection = database.Collection(userBucketName)

	findDocument, err := GetIdDoc(objId)
	if err != nil {
		return err
	}
	updateDocument := GetUpdateDoc(primitive.M{filename: newname})

	one := userFilesCollection.FindOneAndUpdate(context.TODO(), findDocument, updateDocument)
	if one == nil {
		return errors.New("Unexpected nil result during update")
	}
	if one.Err() != nil {
		return one.Err()
	}
	return nil
}

func (r *UserFileRepository) UpdatePublished(userBucketName string, objId string, sevValPublished bool) (*UserFileDto, error) {
	database := utils.GetMongoDatabase(r.mongo)
	var collection *mongo.Collection = database.Collection(userBucketName)

	findDocument, err := GetIdDoc(objId)
	if err != nil {
		return nil, err
	}

	updateDocument := GetUpdateDoc(primitive.M{published: sevValPublished})

	one := collection.FindOneAndUpdate(context.TODO(), findDocument, updateDocument)
	if one == nil {
		return nil, errors.New("Unexpected nil result during update")
	}
	if one.Err() != nil {
		return nil, one.Err()
	}
	var elem UserFileDto
	if err := one.Decode(&elem); err != nil {
		return nil, err
	}
	return &elem, nil
}

func IsDocumentExists(mongoC *mongo.Client, collection string, request interface{}, opts ...*options.FindOneOptions) (bool, error) {
	database := utils.GetMongoDatabase(mongoC)

	// https://siongui.github.io/2017/03/13/go-pass-slice-or-array-as-variadic-parameter/#id12
	res := database.Collection(collection).FindOne(context.TODO(), request, opts[:]...)
	if res.Err() != nil {
		if res.Err() == mongo.ErrNoDocuments {
			return false, nil
		}
		Logger.Errorf("Error during find '%v' : %v", request, res.Err())
		return false, res.Err()
	}

	_, e := res.DecodeBytes()

	if e != nil {
		Logger.Errorf("Error during DecodeBytes '%v' : %v", request, res.Err())
		return false, e
	} else {
		return true, nil
	}
}
