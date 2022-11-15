/**
 * @Author: zhangchao
 * @Description:
 * @Date: 2022/11/15 3:46 PM
 */
package transport

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	provider "github.com/jordan-rash/wasmcloud-provider"
	"github.com/nats-io/nats.go"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/wasmcloud/actor-tinygo"
)

type Transport interface {
	Send(msg actor.Message) ([]byte, error)
}

// ProviderTransport sends messages to a HttpServer service
// HttpServer is the contract to be implemented by actor
type ProviderTransport struct {
	LD             provider.LinkDefinition
	Timeout        time.Duration
	NatsConnection *nats.Conn
	HostData       provider.HostData
	//transport      Transport
}

func NewTransport(ld provider.LinkDefinition, nc *nats.Conn, hostData provider.HostData) *ProviderTransport {
	return &ProviderTransport{LD: ld, NatsConnection: nc, HostData: hostData}
}

func (s *ProviderTransport) Send(msg actor.Message) ([]byte, error) {
	from := s.LD.ProviderEntity()
	to := s.LD.ActorEntity()
	//topic := rpcTopic(to, "default") // todo fix lattice
	guid := GenGuid()
	invocation := provider.Invocation{
		Origin:        from,
		Target:        to,
		Operation:     msg.Method,
		Msg:           msg.Arg,
		ID:            guid,
		HostID:        s.HostData.HostID,
		ContentLength: uint64(len(msg.Arg)),
	}
	err := invocation.EncodeClaims(s.HostData, guid)
	if err != nil {
		return nil, err
	}
	natsBody, err := msgpack.Marshal(invocation) // todo fix
	// NC Request
	subj := fmt.Sprintf("wasmbus.rpc.%s.%s", s.HostData.LatticeRPCPrefix, s.LD.ActorID)
	res, err := s.NatsConnection.Request(subj, natsBody, 5*time.Second)
	if err != nil {
		return nil, err
	}
	ir := provider.InvocationResponse{}
	err = msgpack.Unmarshal(res.Data, &ir)
	if err != nil {
		return nil, err
	}
	return ir.Msg, nil
}

func rpcTopic(entity provider.WasmCloudEntity, latticePrefix string) string {
	if len(entity.LinkName) > 0 {
		// send to provider
		return fmt.Sprintf("wasmbus.rpc.%s.%s.%s", latticePrefix, entity.PublicKey, entity.LinkName)
	} else {
		// send to actor
		return fmt.Sprintf("wasmbus.rpc.%s.%s", latticePrefix, entity.PublicKey)
	}
}

func GenGuid() string {
	uuidWithHyphen := uuid.New()
	uuid := strings.Replace(uuidWithHyphen.String(), "-", "", -1)
	return uuid
}
