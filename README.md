### Introduction

This is a wasmcloud http provider, which lets dapr applications communicate with wasmcloud actors.
![image](https://user-images.githubusercontent.com/13532065/201837261-bbb77966-b5eb-4138-8bd2-e9512d5905e0.png)

### How to Run

#### Prerequisites

You are supposed to know what dapr and wasmcloud is.

#### steps
##### Run wasmcloud

##### Run cousul
You can start consul by running the following command for development purposes:


```shell
consul agent -dev -bind 127.0.0.1 -ui
```

##### Run dapr example
I use the [official service invocation example](https://docs.dapr.io/getting-started/quickstarts/serviceinvocation-quickstart/) form dapr

```shell
daprd -app-id=checkout -app-protocol=http --dapr-http-port=3500 -config=/{path to this project}/test/config/component.yaml
```


##### Run wasmcloud 
Run provider(this project), and wasmcloud actor, here I use the Echo.
* run actor `wasmcloud.azurecr.io/echo:0.3.4`
* run this provider, you can complie form source or use `docker.io/docker4zc/dapr-provider-go:0.0.2`(this image can only run on linux), you should config its configuration `{"resolver_address":"http://127.0.0.1:8500","external_address":"127.0.0.1"}`
* define a link between actor and provider, with values `address=0.0.0.0:8888,unique_id=order-processor`

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
```shell
Order passed:  {"body":[123,34,111,114,100,101,114,73,100,34,58,49,125],"method":"POST","path":"orders","query_string":""}
```
### To Do

Finish wasmcloud actor call dapr service.


