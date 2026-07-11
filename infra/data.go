package infra

import (
	"context"
	"encoding/json"
	"time"

	"github.com/CycleZero/Reimbee/conf"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Data struct {
	DB          *gorm.DB
	RedisClient *RedisClient
}

type RedisClient struct {
	*redis.Client
}

// GetObject 从 Redis 获取并反序列化为目标对象
func (r *RedisClient) GetObject(ctx context.Context, key string, target any) error {
	res, err := r.Get(ctx, key).Result()
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(res), target)
}

// PutObject 序列化对象并存入 Redis
func (r *RedisClient) PutObject(ctx context.Context, key string, target any, expiration time.Duration) error {
	str, err := json.Marshal(target)
	if err != nil {
		return err
	}
	return r.SetEx(ctx, key, string(str), expiration).Err()
}

func NewData(vc *viper.Viper, rdb *RedisClient) *Data {
	host := vc.GetString("data.db.host")
	port := vc.GetString("data.db.port")
	user := vc.GetString("data.db.user")
	password := vc.GetString("data.db.password")
	dbname := vc.GetString("data.db.db_name")
	dsn := getDsn(host, port, user, password, dbname)

	clogger, _ := zap.NewDevelopment()
	masterDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger: func() logger.Interface {
			if conf.EnableDBDebug() {
				return logger.Default.LogMode(logger.Info)
			}
			return nil
		}(),
	})
	if err != nil {
		clogger.Fatal("连接数据库失败", zap.Error(err))
	}

	return &Data{
		DB:          masterDB,
		RedisClient: rdb,
	}
}

func getDsn(host, port, user, password, dbname string) string {
	return user + ":" + password + "@tcp(" + host + ":" + port + ")/" + dbname + "?charset=utf8mb4&parseTime=True&loc=Local"
}

func NewRedisClient(vc *viper.Viper) *redis.Client {
	host := vc.GetString("data.redis.host")
	port := vc.GetString("data.redis.port")
	password := vc.GetString("data.redis.password")
	rdb := redis.NewClient(&redis.Options{
		Addr:     host + ":" + port,
		Password: password,
		DB:       0,
	})
	return rdb
}

func NewCustomRedisClient(rdb *redis.Client) *RedisClient {
	return &RedisClient{rdb}
}

// GetDB 从 Data 中获取 *gorm.DB 供 Wire 注入
func GetDB(data *Data) *gorm.DB {
	return data.DB
}
