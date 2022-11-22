### Introduction

This is a wasmcloud http provider, which lets dapr applications communicate with wasmcloud actors.
![image](https://user-images.githubusercontent.com/13532065/201837261-bbb77966-b5eb-4138-8bd2-e9512d5905e0.png)

### How to Run

#### Prerequisites

You are supposed to know what dapr and wasmcloud is.

#### steps
##### Run wasmcloud

##### Run consul
You can start consul by running the following command for development purposes:

```shell
consul agent -dev -bind 127.0.0.1 -ui
```

##### Run dapr example
I use the [official service invocation example](https://docs.dapr.io/getting-started/quickstarts/serviceinvocation-quickstart/) form dapr

build and run order-processor app and its daprd sidecar
```shell
daprd -app-id=order-processor -app-port=6001 -app-protocol=http --dapr-grpc-port=50002 --metrics-port=9999 --dapr-http-port=3501  -config=/{path to this project}/test/config/component.yaml

# cd to the quickstarts/service_invocation/go/http/order-processor directory
go run app.go
# or build and run the binary
```

Run checkout's daprd sidecar
```shell
daprd -app-id=checkout -app-protocol=http --dapr-http-port=3500 -config=/{path to this project}/test/config/component.yaml
```


##### Run wasmcloud Actor
Run provider(this project), and wasmcloud actor, here I use the Echo.
* run actor `docker.io/docker4zc/dapr-wasm-actor:0.1.0`
  ![image-20221122164108329](https://image-1255620078.cos.ap-nanjing.myqcloud.com/image-20221122164108329.png)
* run this provider, you can complie form source or use `docker.io/docker4zc/dapr-provider-go:0.0.4`(this image can only run on linux), you should config its configuration `{"resolver_address":"http://127.0.0.1:8500","external_address":"127.0.0.1"}`
  ![image-20221122165710560](https://image-1255620078.cos.ap-nanjing.myqcloud.com/image-20221122165710560.png)
* define a link between actor and provider, with values `address=0.0.0.0:8888,unique_id=wasm-processor`
  ![image-20221122164343723](https://image-1255620078.cos.ap-nanjing.myqcloud.com/image-20221122164343723.png)

##### Run dapr app to call 
We only need to start checkout app.
```shell
git clone https://github.com/dapr/quickstarts.git
cd service_invocation/go/http/checkout
go build .
./checkout
```
##### Result
You are supposed to see the request info on the screen, as the echo actor will return the request info.
```
Order passed:  {"Proxy": "by wasmcloud", "origin":{"orderId":1}}
Order passed:  {"Proxy": "by wasmcloud", "origin":{"orderId":2}}
Order passed:  {"Proxy": "by wasmcloud", "origin":{"orderId":3}}
Order passed:  {"Proxy": "by wasmcloud", "origin":{"orderId":4}}
Order passed:  {"Proxy": "by wasmcloud", "origin":{"orderId":5}}
Order passed:  {"Proxy": "by wasmcloud", "origin":{"orderId":6}}
Order passed:  {"Proxy": "by wasmcloud", "origin":{"orderId":7}}
Order passed:  {"Proxy": "by wasmcloud", "origin":{"orderId":8}}
Order passed:  {"Proxy": "by wasmcloud", "origin":{"orderId":9}}
Order passed:  {"Proxy": "by wasmcloud", "origin":{"orderId":10}}
```
And on the order-processor screen, you can see the request info:
```
Order received :  {"Proxy": "by wasmcloud", "origin":{"orderId":1}}
Order received :  {"Proxy": "by wasmcloud", "origin":{"orderId":2}}
Order received :  {"Proxy": "by wasmcloud", "origin":{"orderId":3}}
Order received :  {"Proxy": "by wasmcloud", "origin":{"orderId":4}}
Order received :  {"Proxy": "by wasmcloud", "origin":{"orderId":5}}
Order received :  {"Proxy": "by wasmcloud", "origin":{"orderId":6}}
Order received :  {"Proxy": "by wasmcloud", "origin":{"orderId":7}}
Order received :  {"Proxy": "by wasmcloud", "origin":{"orderId":8}}
Order received :  {"Proxy": "by wasmcloud", "origin":{"orderId":9}}
Order received :  {"Proxy": "by wasmcloud", "origin":{"orderId":10}}
```
### To Do

Add Mtls and more feature support


