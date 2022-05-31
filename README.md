This exercise helps me to explore
* the dgraph database
* the golang programing language

This program simulates a Race condition on a Dgraph database and confirm that the ACID-transaction support of the database will protect the database consistency.  

2 go routines will try to increase the balance of the same wallet
at the same time on 2 different transactions. Each go routines will
```
1) create a new database's transaction
2) read current wallet balance
<wait for a signal>
3) compute new balance (base on the current balance) 
4) update wallet balance
5) commit the database's transaction
```

Both go routine will wait for my signal after the step (2). So both goroutine will race to update the wallet balance.

as expected! dgraph did not fall to this race condition
* 1 go routine will pass (successfully update the wallet balance)
* other will fail

# Understand the Data model

In the `schema.graphql` we have 
* a `wallet` structure 
* and a `P2P` structure (a P2P means a transfer from a sender wallet to a receiver wallet)
  * When we create a new P2P, we will update the balances of the wallet sender and receiver. All in the same transaction.

# Play with schema

import the `schema.graphql` to the database 
```bash
curl -X POST localhost:8080/admin/schema --data-binary '@schema.graphql'
```

To drop all data and schema:
```bash
curl -X POST localhost:8080/alter -d '{"drop_all": true}'
```

To drop all data only (keep schema):
```bash
curl -X POST localhost:8080/alter -d '{"drop_op": "DATA"}'
```

# Play with graphql

create 2 wallets "a" and "b"

```graphql
mutation Create2Wallets {
  walletA: addWallet(input: {alias: "a", balance: 100}) {
    numUids
    wallet {
      alias
      balance
    }
  }
  walletB: addWallet(input: {alias: "b", balance: 100}) {
    numUids
    wallet {
      alias
      balance
    }
  }
}
```

create a P2P

```graphql
mutation CreateP2P {
  addP2P(input: {from: {alias: "a"}, to: {alias: "b"}, amount: 3}) {
    p2P {
      id
      from {
        alias
        balance
      }
      to {
        alias
        balance
      }
    }
  }
  sender: updateWallet(input: {filter: {alias: {eq: "a"}}, set: {balance: 97}}) {
    wallet {
      balance
    }
  }
  receiver: updateWallet(input: {filter: {alias: {eq: "a"}}, set: {balance: 103}}) {
    wallet {
      balance
    }
  }
}
```

query for wallets balance

```graphql
fragment infoWalletBalance on Wallet {
  alias
  balance
}
query getBalances($walletA: String!, $walletB: String!) {
  walletA: getWallet(alias: $walletA) {
    ...infoWalletBalance
  }
  walletB: getWallet(alias: $walletB) {
    ...infoWalletBalance
  }
}
```

# Play with dql

I mostly play in the [ratel interface](https://play.dgraph.io/) or Insomnia. 

get wallet balance

```dql
curl localhost:8080/query -H 'Content-Type: application/dql' -d '
{
  q(func: eq(Wallet.alias, "a")){
    alias: Wallet.alias
    balance: Wallet.balance
  }
}'
```

set wallet balance with upsert
 * we query the uid of the wallet a then use it to update its balance


```dql
curl localhost:8080/mutate?commitNow=true -H 'Content-Type: application/json' -d '
{
    "query": "{q(func: eq(Wallet.alias, \"a\")) {v as uid}}",
    "set": {
        "uid": "uid(v)",
        "Wallet.balance": 204
    }
}
'
```

# Notes

* in dgraph almost everything are nodes and edges (or predicats) 
  * a Wallet is a node, and a node contains only a `uid` and no other data
  * the Wallet balance is not a property of the node Wallet, but rather an edge which links the Wallet uid to a float value (value node). 
* It makes me puzzle that Dgraph uses 2 different query language systems: 
  * GraphQL (standard-specs compliant, supplement with specific dgraph's directive) 
  * DQL (inspired from GraphQL)
* IMO we have to learn DQL
  * GraphQL support is just a plus, but not enough for me.
    * We can conveniently plug the Frontend application directly to the Dgraph database without passing through the Server application layer.
    * However, this kind of applications are limited, We aim for more complex enterprise grade applications so, A application server is a requirement to execute Business Logic. 
  * The SDK client connect to Dgraph via gRPC and use the DQL. We can also connect to Dgraph via HTTP and use the GraphQL but it is less efficient than the first options: 
    * gRPC is better than HTTP (for speed and memory)
    * DQL is more powerful than GraphQL
    * We can use the SDK (so DQL) to create new ACID Transaction


