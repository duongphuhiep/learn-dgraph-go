package main

import (
	"context"
	"encoding/json"
	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"log"
	"sync"
)

var cond = sync.NewCond(&sync.Mutex{})
var waitGroupWrite = sync.WaitGroup{}
var waitGroupRead = sync.WaitGroup{}
var dgraphCloudEndpoint string
var dgraphKey string

func main() {
	err := loadConfig()
	if err != nil {
		log.Fatal("Unable to load config. Verify your config.yaml. ", err)
	}

	waitGroupWrite.Add(2)
	waitGroupRead.Add(2)

	go func() {
		err := IncreaseBalance(1)
		if err != nil {
			log.Fatal(err)
		}
	}()
	go func() {
		err := IncreaseBalance(2)
		if err != nil {
			log.Fatal(err)
		}
	}()

	waitGroupRead.Wait()
	cond.Broadcast()
	waitGroupWrite.Wait()

	log.Println("Finished")
}

func loadConfig() error {
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	if err != nil {
		return err
	}
	dgraphCloudEndpoint = viper.GetString("dgraph.cloud_endpoint")
	dgraphKey = viper.GetString("dgraph.key")
	return nil
}

func IncreaseBalance(delta float64) error {
	cond.L.Lock()

	conn, err := dgo.DialCloud(dgraphCloudEndpoint, dgraphKey)
	if err != nil {
		return err
	}
	defer func(conn *grpc.ClientConn) {
		err := conn.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(conn)

	ctx := context.Background()
	client := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	tnx := client.NewTxn()
	defer func(tnx *dgo.Txn, ctx context.Context) {
		err := tnx.Discard(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}(tnx, ctx)

	balance, err := getWalletBalance(tnx, ctx)
	if err != nil {
		return err
	}
	log.Printf("Current balance is %f", balance)

	waitGroupRead.Done()
	cond.Wait()

	newBalance := balance + delta
	_, err = setWalletBalance(tnx, ctx, newBalance)
	if err != nil {
		return err
	}

	err = tnx.Commit(ctx)
	if err != nil {
		return err
	}
	log.Printf("New balance is %f", newBalance)

	cond.L.Unlock()
	waitGroupWrite.Done()
	return nil
}

func setWalletBalance(tnx *dgo.Txn, ctx context.Context, newBalance float64) (*api.Response, error) {
	var mu struct {
		Uid     string  `json:"uid"`
		Balance float64 `json:"Wallet.balance"`
	}
	mu.Uid = "uid(v)"
	mu.Balance = newBalance
	muBytes, err := json.Marshal(mu)
	if err != nil {
		return nil, err
	}

	req := &api.Request{
		Query: `
		{
			q(func: eq(Wallet.alias, "a")) {
				v as uid
			}
		}`,
		Mutations: []*api.Mutation{{
			SetJson:   muBytes,
			CommitNow: false,
		}},
		CommitNow: false,
	}

	return tnx.Do(ctx, req)
}

func getWalletBalance(tnx *dgo.Txn, ctx context.Context) (float64, error) {
	const queryString = `
		{
		  q(func: eq(Wallet.alias, "a")){
			alias: Wallet.alias
			balance: Wallet.balance
		  }
		}`
	rawResp, err := tnx.Query(ctx, queryString)
	if err != nil {
		return -1, err
	}

	var resp struct {
		Q []struct {
			Alias   string
			Balance float64
		}
	}

	if err := json.Unmarshal(rawResp.GetJson(), &resp); err != nil {
		return -1, err
	}

	return resp.Q[0].Balance, nil
}
