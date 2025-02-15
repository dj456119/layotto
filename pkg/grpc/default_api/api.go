/*
 * Copyright 2021 Layotto Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package default_api

import (
	"context"
	"errors"
	"fmt"
	"github.com/dapr/components-contrib/bindings"
	"github.com/dapr/components-contrib/secretstores"
	"io"
	l8_comp_pubsub "mosn.io/layotto/components/pubsub"
	"strings"
	"sync"

	grpc_api "mosn.io/layotto/pkg/grpc"
	"mosn.io/layotto/pkg/grpc/dapr"
	dapr_common_v1pb "mosn.io/layotto/pkg/grpc/dapr/proto/common/v1"
	dapr_v1pb "mosn.io/layotto/pkg/grpc/dapr/proto/runtime/v1"
	mgrpc "mosn.io/mosn/pkg/filter/network/grpc"

	"mosn.io/pkg/utils"

	_ "net/http/pprof"

	"github.com/dapr/components-contrib/state"
	"github.com/golang/protobuf/ptypes/empty"
	"mosn.io/layotto/components/file"

	"mosn.io/layotto/pkg/converter"
	runtime_lock "mosn.io/layotto/pkg/runtime/lock"
	runtime_sequencer "mosn.io/layotto/pkg/runtime/sequencer"

	contrib_contenttype "github.com/dapr/components-contrib/contenttype"
	"github.com/dapr/components-contrib/pubsub"
	contrib_pubsub "github.com/dapr/components-contrib/pubsub"
	"github.com/google/uuid"
	jsoniter "github.com/json-iterator/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"mosn.io/layotto/components/configstores"
	"mosn.io/layotto/components/hello"
	"mosn.io/layotto/components/lock"
	"mosn.io/layotto/components/rpc"
	"mosn.io/layotto/components/sequencer"
	"mosn.io/layotto/pkg/messages"
	runtimev1pb "mosn.io/layotto/spec/proto/runtime/v1"
	"mosn.io/pkg/log"
)

const (
	Metadata_key_pubsubName = "pubsubName"
)

var (
	ErrNoInstance = errors.New("no instance found")
	bytesPool     = sync.Pool{
		New: func() interface{} {
			// set size to 100kb
			return new([]byte)
		},
	}
	// FIXME I put it here for compatibility.Don't write singleton like this !
	// It should be refactored and deleted.
	LayottoAPISingleton API
)

type API interface {
	SayHello(ctx context.Context, in *runtimev1pb.SayHelloRequest) (*runtimev1pb.SayHelloResponse, error)
	// InvokeService do rpc calls.
	InvokeService(ctx context.Context, in *runtimev1pb.InvokeServiceRequest) (*runtimev1pb.InvokeResponse, error)
	// GetConfiguration gets configuration from configuration store.
	GetConfiguration(context.Context, *runtimev1pb.GetConfigurationRequest) (*runtimev1pb.GetConfigurationResponse, error)
	// SaveConfiguration saves configuration into configuration store.
	SaveConfiguration(context.Context, *runtimev1pb.SaveConfigurationRequest) (*emptypb.Empty, error)
	// DeleteConfiguration deletes configuration from configuration store.
	DeleteConfiguration(context.Context, *runtimev1pb.DeleteConfigurationRequest) (*emptypb.Empty, error)
	// SubscribeConfiguration gets configuration from configuration store and subscribe the updates.
	SubscribeConfiguration(runtimev1pb.Runtime_SubscribeConfigurationServer) error
	// Publishes events to the specific topic.
	PublishEvent(context.Context, *runtimev1pb.PublishEventRequest) (*emptypb.Empty, error)
	// State
	GetState(ctx context.Context, in *runtimev1pb.GetStateRequest) (*runtimev1pb.GetStateResponse, error)
	GetBulkState(ctx context.Context, in *runtimev1pb.GetBulkStateRequest) (*runtimev1pb.GetBulkStateResponse, error)
	SaveState(ctx context.Context, in *runtimev1pb.SaveStateRequest) (*emptypb.Empty, error)
	DeleteState(ctx context.Context, in *runtimev1pb.DeleteStateRequest) (*emptypb.Empty, error)
	DeleteBulkState(ctx context.Context, in *runtimev1pb.DeleteBulkStateRequest) (*emptypb.Empty, error)
	ExecuteStateTransaction(ctx context.Context, in *runtimev1pb.ExecuteStateTransactionRequest) (*emptypb.Empty, error)
	// Get File
	GetFile(*runtimev1pb.GetFileRequest, runtimev1pb.Runtime_GetFileServer) error
	// Put file with stream.
	PutFile(runtimev1pb.Runtime_PutFileServer) error
	// List all files
	ListFile(ctx context.Context, in *runtimev1pb.ListFileRequest) (*runtimev1pb.ListFileResp, error)
	//Delete specific file
	DelFile(ctx context.Context, in *runtimev1pb.DelFileRequest) (*emptypb.Empty, error)
	// Get file meta data, if file not exist,return code.NotFound error
	GetFileMeta(ctx context.Context, in *runtimev1pb.GetFileMetaRequest) (*runtimev1pb.GetFileMetaResponse, error)
	// Distributed Lock API
	TryLock(context.Context, *runtimev1pb.TryLockRequest) (*runtimev1pb.TryLockResponse, error)
	Unlock(context.Context, *runtimev1pb.UnlockRequest) (*runtimev1pb.UnlockResponse, error)
	// Sequencer API
	GetNextId(context.Context, *runtimev1pb.GetNextIdRequest) (*runtimev1pb.GetNextIdResponse, error)
	// InvokeBinding Binding API
	InvokeBinding(context.Context, *runtimev1pb.InvokeBindingRequest) (*runtimev1pb.InvokeBindingResponse, error)
	// Gets secrets from secret stores.
	GetSecret(context.Context, *runtimev1pb.GetSecretRequest) (*runtimev1pb.GetSecretResponse, error)
	// Gets a bulk of secrets
	GetBulkSecret(context.Context, *runtimev1pb.GetBulkSecretRequest) (*runtimev1pb.GetBulkSecretResponse, error)
	// GrpcAPI related
	grpc_api.GrpcAPI
}

// api is a default implementation for MosnRuntimeServer.
type api struct {
	daprAPI                  dapr.DaprGrpcAPI
	appId                    string
	hellos                   map[string]hello.HelloService
	configStores             map[string]configstores.Store
	rpcs                     map[string]rpc.Invoker
	pubSubs                  map[string]pubsub.PubSub
	stateStores              map[string]state.Store
	transactionalStateStores map[string]state.TransactionalStore
	fileOps                  map[string]file.File
	lockStores               map[string]lock.LockStore
	sequencers               map[string]sequencer.Store
	sendToOutputBindingFn    func(name string, req *bindings.InvokeRequest) (*bindings.InvokeResponse, error)
	secretStores             map[string]secretstores.SecretStore
	// app callback
	AppCallbackConn   *grpc.ClientConn
	topicPerComponent map[string]TopicSubscriptions
	// json
	json jsoniter.API
}

func (a *api) Init(conn *grpc.ClientConn) error {
	// 1. set connection
	a.AppCallbackConn = conn
	return a.startSubscribing()
}

func (a *api) Register(s *grpc.Server, registeredServer mgrpc.RegisteredServer) (mgrpc.RegisteredServer, error) {
	LayottoAPISingleton = a
	runtimev1pb.RegisterRuntimeServer(s, a)
	return registeredServer, nil
}

func NewGrpcAPI(ac *grpc_api.ApplicationContext) grpc_api.GrpcAPI {
	return NewAPI(ac.AppId,
		ac.Hellos, ac.ConfigStores, ac.Rpcs, ac.PubSubs, ac.StateStores, ac.Files, ac.LockStores, ac.Sequencers,
		ac.SendToOutputBindingFn, ac.SecretStores)
}

func NewAPI(
	appId string,
	hellos map[string]hello.HelloService,
	configStores map[string]configstores.Store,
	rpcs map[string]rpc.Invoker,
	pubSubs map[string]pubsub.PubSub,
	stateStores map[string]state.Store,
	files map[string]file.File,
	lockStores map[string]lock.LockStore,
	sequencers map[string]sequencer.Store,
	sendToOutputBindingFn func(name string, req *bindings.InvokeRequest) (*bindings.InvokeResponse, error),
	secretStores map[string]secretstores.SecretStore,
) API {
	// filter out transactionalStateStores
	transactionalStateStores := map[string]state.TransactionalStore{}
	for key, store := range stateStores {
		if state.FeatureTransactional.IsPresent(store.Features()) {
			transactionalStateStores[key] = store.(state.TransactionalStore)
		}
	}
	dAPI := dapr.NewDaprServer(appId, hellos, configStores, rpcs, pubSubs,
		stateStores, transactionalStateStores,
		files, lockStores, sequencers, sendToOutputBindingFn, secretStores)
	// construct
	return &api{
		daprAPI:                  dAPI,
		appId:                    appId,
		hellos:                   hellos,
		configStores:             configStores,
		rpcs:                     rpcs,
		pubSubs:                  pubSubs,
		stateStores:              stateStores,
		transactionalStateStores: transactionalStateStores,
		fileOps:                  files,
		lockStores:               lockStores,
		sequencers:               sequencers,
		sendToOutputBindingFn:    sendToOutputBindingFn,
		secretStores:             secretStores,
		json:                     jsoniter.ConfigFastest,
	}
}

func (a *api) SayHello(ctx context.Context, in *runtimev1pb.SayHelloRequest) (*runtimev1pb.SayHelloResponse, error) {
	h, err := a.getHello(in.ServiceName)
	if err != nil {
		log.DefaultLogger.Errorf("[runtime] [grpc.say_hello] get hello error: %v", err)
		return nil, err
	}
	// create hello request based on pb.go struct
	req := &hello.HelloRequest{
		Name: in.Name,
	}
	resp, err := h.Hello(ctx, req)
	if err != nil {
		log.DefaultLogger.Errorf("[runtime] [grpc.say_hello] request hello error: %v", err)
		return nil, err
	}
	// create response base on hello.Response
	return &runtimev1pb.SayHelloResponse{
		Hello: resp.HelloString,
		Data:  in.Data,
	}, nil

}

func (a *api) getHello(name string) (hello.HelloService, error) {
	if len(a.hellos) == 0 {
		return nil, ErrNoInstance
	}
	h, ok := a.hellos[name]
	if !ok {
		return nil, ErrNoInstance
	}
	return h, nil
}

func (a *api) InvokeService(ctx context.Context, in *runtimev1pb.InvokeServiceRequest) (*runtimev1pb.InvokeResponse, error) {
	// convert request
	var msg *dapr_common_v1pb.InvokeRequest = nil
	if in != nil && in.Message != nil {
		msg = &dapr_common_v1pb.InvokeRequest{
			Method:      in.Message.Method,
			Data:        in.Message.Data,
			ContentType: in.Message.ContentType,
		}
		if in.Message.HttpExtension != nil {
			msg.HttpExtension = &dapr_common_v1pb.HTTPExtension{
				Verb:        dapr_common_v1pb.HTTPExtension_Verb(in.Message.HttpExtension.Verb),
				Querystring: in.Message.HttpExtension.Querystring,
			}
		}
	}
	// delegate to dapr api implementation
	daprResp, err := a.daprAPI.InvokeService(ctx, &dapr_v1pb.InvokeServiceRequest{
		Id:      in.Id,
		Message: msg,
	})
	// handle error
	if err != nil {
		return nil, err
	}

	// convert resp
	return &runtimev1pb.InvokeResponse{
		Data:        daprResp.Data,
		ContentType: daprResp.ContentType,
	}, nil
}

// GetConfiguration gets configuration from configuration store.
func (a *api) GetConfiguration(ctx context.Context, req *runtimev1pb.GetConfigurationRequest) (*runtimev1pb.GetConfigurationResponse, error) {
	resp := &runtimev1pb.GetConfigurationResponse{}
	// check store type supported or not
	store, ok := a.configStores[req.StoreName]
	if !ok {
		return nil, errors.New(fmt.Sprintf("configure store [%+v] don't support now", req.StoreName))
	}
	//here protect user use space for sting, eg: " ", "de fault"
	if strings.ReplaceAll(req.Group, " ", "") == "" {
		req.Group = store.GetDefaultGroup()
	}
	if strings.ReplaceAll(req.Label, " ", "") == "" {
		req.Label = store.GetDefaultLabel()
	}
	items, err := store.Get(ctx, &configstores.GetRequest{AppId: req.AppId, Group: req.Group, Label: req.Label, Keys: req.Keys, Metadata: req.Metadata})
	if err != nil {
		return nil, errors.New(fmt.Sprintf("get configuration failed with error: %+v", err))
	}
	for _, item := range items {
		resp.Items = append(resp.Items, &runtimev1pb.ConfigurationItem{Group: item.Group, Label: item.Label, Key: item.Key, Content: item.Content, Tags: item.Tags, Metadata: item.Metadata})
	}
	return resp, err
}

// SaveConfiguration saves configuration into configuration store.
func (a *api) SaveConfiguration(ctx context.Context, req *runtimev1pb.SaveConfigurationRequest) (*emptypb.Empty, error) {
	store, ok := a.configStores[req.StoreName]
	if !ok {
		return nil, errors.New(fmt.Sprintf("configure store [%+v] don't support now", req.StoreName))
	}
	setReq := &configstores.SetRequest{}
	setReq.AppId = req.AppId
	setReq.StoreName = req.StoreName

	for index, item := range req.Items {
		if strings.ReplaceAll(item.Group, " ", "") == "" {
			req.Items[index].Group = store.GetDefaultGroup()
		}
		if strings.ReplaceAll(item.Label, " ", "") == "" {
			req.Items[index].Label = store.GetDefaultLabel()
		}
		setReq.Items = append(setReq.Items, &configstores.ConfigurationItem{Group: item.Group, Label: item.Label, Key: item.Key, Content: item.Content, Tags: item.Tags, Metadata: item.Metadata})
	}
	err := store.Set(ctx, setReq)
	return &emptypb.Empty{}, err
}

// DeleteConfiguration deletes configuration from configuration store.
func (a *api) DeleteConfiguration(ctx context.Context, req *runtimev1pb.DeleteConfigurationRequest) (*emptypb.Empty, error) {
	store, ok := a.configStores[req.StoreName]
	if !ok {
		return nil, errors.New(fmt.Sprintf("configure store [%+v] don't support now", req.StoreName))
	}
	if strings.ReplaceAll(req.Group, " ", "") == "" {
		req.Group = store.GetDefaultGroup()
	}
	if strings.ReplaceAll(req.Label, " ", "") == "" {
		req.Label = store.GetDefaultLabel()
	}
	err := store.Delete(ctx, &configstores.DeleteRequest{AppId: req.AppId, Group: req.Group, Label: req.Label, Keys: req.Keys, Metadata: req.Metadata})
	return &emptypb.Empty{}, err
}

// SubscribeConfiguration gets configuration from configuration store and subscribe the updates.
func (a *api) SubscribeConfiguration(sub runtimev1pb.Runtime_SubscribeConfigurationServer) error {
	wg := sync.WaitGroup{}
	wg.Add(2)
	var subErr error
	respCh := make(chan *configstores.SubscribeResp)
	recvExitCh := make(chan struct{})
	subscribedStore := make([]configstores.Store, 0, 1)
	// TODO currently this goroutine model is error-prone,and it should be refactored after new version of configuration API being accepted
	// 1. start a reader goroutine
	utils.GoWithRecover(func() {
		defer wg.Done()
		for {
			// 1.1. read stream
			req, err := sub.Recv()
			// 1.2. if an error happens,stop all the subscribers
			if err != nil {
				log.DefaultLogger.Errorf("occur error in subscribe, err: %+v", err)
				// stop all the subscribers
				for _, store := range subscribedStore {
					// TODO this method will stop subscribers created by other connections.Should be refactored
					store.StopSubscribe()
				}
				subErr = err
				// stop writer goroutine
				close(recvExitCh)
				return
			}
			// 1.3. else find the component and delegate to it
			store, ok := a.configStores[req.StoreName]
			// 1.3.1. stop if StoreName is not supported
			if !ok {
				log.DefaultLogger.Errorf("configure store [%+v] don't support now", req.StoreName)
				// stop all the subscribers
				for _, store := range subscribedStore {
					store.StopSubscribe()
				}
				subErr = errors.New(fmt.Sprintf("configure store [%+v] don't support now", req.StoreName))
				// stop writer goroutine
				close(recvExitCh)
				return
			}
			// 1.3.2. use default settings if blank
			if strings.ReplaceAll(req.Group, " ", "") == "" {
				req.Group = store.GetDefaultGroup()
			}
			if strings.ReplaceAll(req.Label, " ", "") == "" {
				req.Label = store.GetDefaultLabel()
			}
			// 1.3.3. delegate to the component
			store.Subscribe(&configstores.SubscribeReq{AppId: req.AppId, Group: req.Group, Label: req.Label, Keys: req.Keys, Metadata: req.Metadata}, respCh)
			subscribedStore = append(subscribedStore, store)
		}
	}, nil)
	// 2. start a writer goroutine
	utils.GoWithRecover(func() {
		defer wg.Done()
		for {
			select {
			// read response from components
			case resp, ok := <-respCh:
				if !ok {
					return
				}
				items := make([]*runtimev1pb.ConfigurationItem, 0, 10)
				for _, item := range resp.Items {
					items = append(items, &runtimev1pb.ConfigurationItem{Group: item.Group, Label: item.Label, Key: item.Key, Content: item.Content, Tags: item.Tags, Metadata: item.Metadata})
				}
				// write to response stream
				sub.Send(&runtimev1pb.SubscribeConfigurationResponse{StoreName: resp.StoreName, AppId: resp.StoreName, Items: items})
			//	read exit signal
			case <-recvExitCh:
				return
			}
		}
	}, nil)
	wg.Wait()
	log.DefaultLogger.Warnf("subscribe gorountine exit")
	return subErr
}

func (a *api) PublishEvent(ctx context.Context, in *runtimev1pb.PublishEventRequest) (*emptypb.Empty, error) {
	result, err := a.doPublishEvent(ctx, in.PubsubName, in.Topic, in.Data, in.DataContentType, in.Metadata)
	if err != nil {
		log.DefaultLogger.Errorf("[runtime] [grpc.PublishEvent] %v", err)
	}
	return result, err
}

// doPublishEvent is a protocal irrelevant function to do event publishing.
// It's easy to add APIs for other protocals.Just move this func to a separate layer if you need.
func (a *api) doPublishEvent(ctx context.Context, pubsubName string, topic string, data []byte, contentType string, metadata map[string]string) (*emptypb.Empty, error) {
	// 1. validate
	if pubsubName == "" {
		err := status.Error(codes.InvalidArgument, messages.ErrPubsubEmpty)
		return &emptypb.Empty{}, err
	}
	if topic == "" {
		err := status.Errorf(codes.InvalidArgument, messages.ErrTopicEmpty, pubsubName)
		return &emptypb.Empty{}, err
	}
	// 2. get component
	component, ok := a.pubSubs[pubsubName]
	if !ok {
		err := status.Errorf(codes.InvalidArgument, messages.ErrPubsubNotFound, pubsubName)
		return &emptypb.Empty{}, err
	}

	// 3. new cloudevent request
	if data == nil {
		data = []byte{}
	}
	var envelope map[string]interface{}
	var err error = nil
	if contrib_contenttype.IsCloudEventContentType(contentType) {
		envelope, err = contrib_pubsub.FromCloudEvent(data, topic, pubsubName, "")
		if err != nil {
			err = status.Errorf(codes.InvalidArgument, messages.ErrPubsubCloudEventCreation, err.Error())
			return &emptypb.Empty{}, err
		}
	} else {
		envelope = contrib_pubsub.NewCloudEventsEnvelope(uuid.New().String(), l8_comp_pubsub.DefaultCloudEventSource, l8_comp_pubsub.DefaultCloudEventType, "", topic, pubsubName,
			contentType, data, "")
	}
	features := component.Features()
	pubsub.ApplyMetadata(envelope, features, metadata)

	b, err := jsoniter.ConfigFastest.Marshal(envelope)
	if err != nil {
		err = status.Errorf(codes.InvalidArgument, messages.ErrPubsubCloudEventsSer, topic, pubsubName, err.Error())
		return &emptypb.Empty{}, err
	}
	// 4. publish
	req := pubsub.PublishRequest{
		PubsubName: pubsubName,
		Topic:      topic,
		Data:       b,
		Metadata:   metadata,
	}

	// TODO limit topic scope
	err = component.Publish(&req)
	if err != nil {
		nerr := status.Errorf(codes.Internal, messages.ErrPubsubPublishMessage, topic, pubsubName, err.Error())
		return &emptypb.Empty{}, nerr
	}
	return &emptypb.Empty{}, nil
}

func (a *api) getStateStore(name string) (state.Store, error) {
	if a.stateStores == nil || len(a.stateStores) == 0 {
		return nil, status.Error(codes.FailedPrecondition, messages.ErrStateStoresNotConfigured)
	}

	if a.stateStores[name] == nil {
		return nil, status.Errorf(codes.InvalidArgument, messages.ErrStateStoreNotFound, name)
	}
	return a.stateStores[name], nil
}

// wrapDaprComponentError parse and wrap error from dapr component
func (a *api) wrapDaprComponentError(err error, format string, args ...interface{}) error {
	e, ok := err.(*state.ETagError)
	if !ok {
		return status.Errorf(codes.Internal, format, args...)
	}
	switch e.Kind() {
	case state.ETagMismatch:
		return status.Errorf(codes.Aborted, format, args...)
	case state.ETagInvalid:
		return status.Errorf(codes.InvalidArgument, format, args...)
	}

	return status.Errorf(codes.Internal, format, args...)
}

func (a *api) GetFile(req *runtimev1pb.GetFileRequest, stream runtimev1pb.Runtime_GetFileServer) error {
	if a.fileOps[req.StoreName] == nil {
		return status.Errorf(codes.InvalidArgument, "not supported store type: %+v", req.StoreName)
	}
	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}
	st := &file.GetFileStu{FileName: req.Name, Metadata: req.Metadata}
	data, err := a.fileOps[req.StoreName].Get(stream.Context(), st)
	if err != nil {
		return status.Errorf(codes.Internal, "get file fail,err: %+v", err)
	}

	buffsPtr := bytesPool.Get().(*[]byte)
	buf := *buffsPtr
	if len(buf) == 0 {
		buf = make([]byte, 102400, 102400)
	}
	defer func() {
		data.Close()
		*buffsPtr = buf
		bytesPool.Put(buffsPtr)
	}()

	for {
		length, err := data.Read(buf)
		if err != nil && err != io.EOF {
			log.DefaultLogger.Warnf("get file fail, err: %+v", err)
			return status.Errorf(codes.Internal, "get file fail,err: %+v", err)
		}
		if err == nil || (err == io.EOF && length != 0) {
			resp := &runtimev1pb.GetFileResponse{Data: buf[:length]}
			if err = stream.Send(resp); err != nil {
				return status.Errorf(codes.Internal, "send file data fail,err: %+v", err)
			}
		}
		if err == io.EOF {
			return nil
		}
	}
}

type putObjectStreamReader struct {
	data   []byte
	server runtimev1pb.Runtime_PutFileServer
}

func newPutObjectStreamReader(data []byte, server runtimev1pb.Runtime_PutFileServer) *putObjectStreamReader {
	return &putObjectStreamReader{data: data, server: server}
}

func (r *putObjectStreamReader) Read(p []byte) (int, error) {
	var count int
	total := len(p)
	for {
		if len(r.data) > 0 {
			n := copy(p[count:], r.data)
			r.data = r.data[n:]
			count += n
			if count == total {
				return count, nil
			}
		}
		req, err := r.server.Recv()
		if err != nil {
			if err != io.EOF {
				log.DefaultLogger.Errorf("recv data from grpc stream fail, err:%+v", err)
			}
			return count, err
		}
		r.data = req.Data
	}
}

func (a *api) PutFile(stream runtimev1pb.Runtime_PutFileServer) error {
	req, err := stream.Recv()
	if err != nil {
		//if client send eof error directly, return nil
		if err == io.EOF {
			return nil
		}
		return status.Errorf(codes.Internal, "receive file data fail: err: %+v", err)
	}

	if a.fileOps[req.StoreName] == nil {
		return status.Errorf(codes.InvalidArgument, "not support store type: %+v", req.StoreName)
	}
	fileReader := newPutObjectStreamReader(req.Data, stream)
	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}
	st := &file.PutFileStu{DataStream: fileReader, FileName: req.Name, Metadata: req.Metadata}
	if err = a.fileOps[req.StoreName].Put(stream.Context(), st); err != nil {
		return status.Errorf(codes.Internal, err.Error())
	}
	stream.SendAndClose(&empty.Empty{})
	return nil
}

//ListFile list all files
func (a *api) ListFile(ctx context.Context, in *runtimev1pb.ListFileRequest) (*runtimev1pb.ListFileResp, error) {
	if in.Request == nil {
		return nil, status.Errorf(codes.InvalidArgument, "request can't be nil")
	}
	if in.Request.Metadata == nil {
		in.Request.Metadata = make(map[string]string)
	}

	if a.fileOps[in.Request.StoreName] == nil {
		return nil, status.Errorf(codes.InvalidArgument, "not support store type: %+v", in.Request.StoreName)
	}
	resp, err := a.fileOps[in.Request.StoreName].List(ctx, &file.ListRequest{DirectoryName: in.Request.Name, PageSize: in.PageSize, Marker: in.Marker, Metadata: in.Request.Metadata})
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	files := make([]*runtimev1pb.FileInfo, 0)
	for _, v := range resp.Files {
		file := &runtimev1pb.FileInfo{}
		file.FileName = v.FileName
		file.LastModified = v.LastModified
		file.Size = v.Size
		file.Metadata = v.Meta
		files = append(files, file)
	}
	return &runtimev1pb.ListFileResp{Files: files, Marker: resp.Marker, IsTruncated: resp.IsTruncated}, nil
}

//DelFile delete specific file
func (a *api) DelFile(ctx context.Context, in *runtimev1pb.DelFileRequest) (*emptypb.Empty, error) {
	errCode := codes.Internal
	if in.Request == nil {
		return nil, status.Errorf(codes.InvalidArgument, "request can't be nil")
	}
	if in.Request.Metadata == nil {
		in.Request.Metadata = make(map[string]string)
	}
	if a.fileOps[in.Request.StoreName] == nil {
		return nil, status.Errorf(codes.InvalidArgument, "not support store type: %+v", in.Request.StoreName)
	}
	err := a.fileOps[in.Request.StoreName].Del(ctx, &file.DelRequest{FileName: in.Request.Name, Metadata: in.Request.Metadata})
	if err != nil {
		if code, ok := FileErrMap2GrpcErr[err]; ok {
			errCode = code
		}
		return nil, status.Errorf(errCode, err.Error())
	}
	return &emptypb.Empty{}, nil
}

//GetFileMeta get meta of file
func (a *api) GetFileMeta(ctx context.Context, in *runtimev1pb.GetFileMetaRequest) (*runtimev1pb.GetFileMetaResponse, error) {
	errCode := codes.Internal
	if in.Request == nil {
		return nil, status.Errorf(codes.InvalidArgument, "request can't be nil")
	}
	if in.Request.Metadata == nil {
		in.Request.Metadata = make(map[string]string)
	}
	if a.fileOps[in.Request.StoreName] == nil {
		return nil, status.Errorf(codes.InvalidArgument, "not support store type: %+v", in.Request.StoreName)
	}
	resp, err := a.fileOps[in.Request.StoreName].Stat(ctx, &file.FileMetaRequest{FileName: in.Request.Name, Metadata: in.Request.Metadata})
	if err != nil {
		if code, ok := FileErrMap2GrpcErr[err]; ok {
			errCode = code
		}
		return nil, status.Errorf(errCode, err.Error())
	}
	meta := &runtimev1pb.FileMeta{}
	meta.Metadata = make(map[string]*runtimev1pb.FileMetaValue)
	for k, v := range resp.Metadata {
		meta.Metadata[k] = &runtimev1pb.FileMetaValue{}
		meta.Metadata[k].Value = append(meta.Metadata[k].Value, v...)
	}
	return &runtimev1pb.GetFileMetaResponse{Size: resp.Size, LastModified: resp.LastModified, Response: meta}, nil
}

func (a *api) TryLock(ctx context.Context, req *runtimev1pb.TryLockRequest) (*runtimev1pb.TryLockResponse, error) {
	// 1. validate
	if a.lockStores == nil || len(a.lockStores) == 0 {
		err := status.Error(codes.FailedPrecondition, messages.ErrLockStoresNotConfigured)
		log.DefaultLogger.Errorf("[runtime] [grpc.TryLock] error: %v", err)
		return &runtimev1pb.TryLockResponse{}, err
	}
	if req.ResourceId == "" {
		err := status.Errorf(codes.InvalidArgument, messages.ErrResourceIdEmpty, req.StoreName)
		return &runtimev1pb.TryLockResponse{}, err
	}
	if req.LockOwner == "" {
		err := status.Errorf(codes.InvalidArgument, messages.ErrLockOwnerEmpty, req.StoreName)
		return &runtimev1pb.TryLockResponse{}, err
	}
	if req.Expire <= 0 {
		err := status.Errorf(codes.InvalidArgument, messages.ErrExpireNotPositive, req.StoreName)
		return &runtimev1pb.TryLockResponse{}, err
	}
	// 2. find store component
	store, ok := a.lockStores[req.StoreName]
	if !ok {
		return &runtimev1pb.TryLockResponse{}, status.Errorf(codes.InvalidArgument, messages.ErrLockStoreNotFound, req.StoreName)
	}
	// 3. convert request
	compReq := converter.TryLockRequest2ComponentRequest(req)
	// modify key
	var err error
	compReq.ResourceId, err = runtime_lock.GetModifiedLockKey(compReq.ResourceId, req.StoreName, a.appId)
	if err != nil {
		log.DefaultLogger.Errorf("[runtime] [grpc.TryLock] error: %v", err)
		return &runtimev1pb.TryLockResponse{}, err
	}
	// 4. delegate to the component
	compResp, err := store.TryLock(compReq)
	if err != nil {
		log.DefaultLogger.Errorf("[runtime] [grpc.TryLock] error: %v", err)
		return &runtimev1pb.TryLockResponse{}, err
	}
	// 5. convert response
	resp := converter.TryLockResponse2GrpcResponse(compResp)
	return resp, nil
}

func (a *api) Unlock(ctx context.Context, req *runtimev1pb.UnlockRequest) (*runtimev1pb.UnlockResponse, error) {
	// 1. validate
	if a.lockStores == nil || len(a.lockStores) == 0 {
		err := status.Error(codes.FailedPrecondition, messages.ErrLockStoresNotConfigured)
		log.DefaultLogger.Errorf("[runtime] [grpc.Unlock] error: %v", err)
		return newInternalErrorUnlockResponse(), err
	}
	if req.ResourceId == "" {
		err := status.Errorf(codes.InvalidArgument, messages.ErrResourceIdEmpty, req.StoreName)
		return newInternalErrorUnlockResponse(), err
	}
	if req.LockOwner == "" {
		err := status.Errorf(codes.InvalidArgument, messages.ErrLockOwnerEmpty, req.StoreName)
		return newInternalErrorUnlockResponse(), err
	}
	// 2. find store component
	store, ok := a.lockStores[req.StoreName]
	if !ok {
		return newInternalErrorUnlockResponse(), status.Errorf(codes.InvalidArgument, messages.ErrLockStoreNotFound, req.StoreName)
	}
	// 3. convert request
	compReq := converter.UnlockGrpc2ComponentRequest(req)
	// modify key
	var err error
	compReq.ResourceId, err = runtime_lock.GetModifiedLockKey(compReq.ResourceId, req.StoreName, a.appId)
	if err != nil {
		log.DefaultLogger.Errorf("[runtime] [grpc.TryLock] error: %v", err)
		return newInternalErrorUnlockResponse(), err
	}
	// 4. delegate to the component
	compResp, err := store.Unlock(compReq)
	if err != nil {
		log.DefaultLogger.Errorf("[runtime] [grpc.Unlock] error: %v", err)
		return newInternalErrorUnlockResponse(), err
	}
	// 5. convert response
	resp := converter.UnlockComp2GrpcResponse(compResp)
	return resp, nil
}

func newInternalErrorUnlockResponse() *runtimev1pb.UnlockResponse {
	return &runtimev1pb.UnlockResponse{
		Status: runtimev1pb.UnlockResponse_INTERNAL_ERROR,
	}
}

func (a *api) GetNextId(ctx context.Context, req *runtimev1pb.GetNextIdRequest) (*runtimev1pb.GetNextIdResponse, error) {
	// 1. validate
	if len(a.sequencers) == 0 {
		err := status.Error(codes.FailedPrecondition, messages.ErrSequencerStoresNotConfigured)
		log.DefaultLogger.Errorf("[runtime] [grpc.GetNextId] error: %v", err)
		return &runtimev1pb.GetNextIdResponse{}, err
	}
	if req.Key == "" {
		err := status.Errorf(codes.InvalidArgument, messages.ErrSequencerKeyEmpty, req.StoreName)
		return &runtimev1pb.GetNextIdResponse{}, err
	}
	// 2. convert
	compReq, err := converter.GetNextIdRequest2ComponentRequest(req)
	if err != nil {
		return &runtimev1pb.GetNextIdResponse{}, err
	}
	// modify key
	compReq.Key, err = runtime_sequencer.GetModifiedSeqKey(compReq.Key, req.StoreName, a.appId)
	if err != nil {
		log.DefaultLogger.Errorf("[runtime] [grpc.GetNextId] error: %v", err)
		return &runtimev1pb.GetNextIdResponse{}, err
	}
	// 3. find store component
	store, ok := a.sequencers[req.StoreName]
	if !ok {
		return &runtimev1pb.GetNextIdResponse{}, status.Errorf(codes.InvalidArgument, messages.ErrSequencerStoreNotFound, req.StoreName)
	}
	var next int64
	// 4. invoke component
	if compReq.Options.AutoIncrement == sequencer.WEAK {
		// WEAK
		next, err = a.getNextIdWithWeakAutoIncrement(ctx, store, compReq)
	} else {
		// STRONG
		next, err = a.getNextIdFromComponent(ctx, store, compReq)
	}
	// 5. convert response
	if err != nil {
		log.DefaultLogger.Errorf("[runtime] [grpc.GetNextId] error: %v", err)
		return &runtimev1pb.GetNextIdResponse{}, err
	}
	return &runtimev1pb.GetNextIdResponse{
		NextId: next,
	}, nil
}

func (a *api) getNextIdWithWeakAutoIncrement(ctx context.Context, store sequencer.Store, compReq *sequencer.GetNextIdRequest) (int64, error) {
	// 1. try to get from cache
	support, next, err := runtime_sequencer.GetNextIdFromCache(ctx, store, compReq)

	if !support {
		// 2. get from component
		return a.getNextIdFromComponent(ctx, store, compReq)
	}
	return next, err
}

func (a *api) getNextIdFromComponent(ctx context.Context, store sequencer.Store, compReq *sequencer.GetNextIdRequest) (int64, error) {
	var next int64
	resp, err := store.GetNextId(compReq)
	if err == nil {
		next = resp.NextId
	}
	return next, err
}

func (a *api) InvokeBinding(ctx context.Context, in *runtimev1pb.InvokeBindingRequest) (*runtimev1pb.InvokeBindingResponse, error) {
	daprResp, err := a.daprAPI.InvokeBinding(ctx, &dapr_v1pb.InvokeBindingRequest{
		Name:      in.Name,
		Data:      in.Data,
		Metadata:  in.Metadata,
		Operation: in.Operation,
	})
	if err != nil {
		return &runtimev1pb.InvokeBindingResponse{}, err
	}
	return &runtimev1pb.InvokeBindingResponse{
		Data:     daprResp.Data,
		Metadata: daprResp.Metadata,
	}, nil
}

func (a *api) GetSecret(ctx context.Context, in *runtimev1pb.GetSecretRequest) (*runtimev1pb.GetSecretResponse, error) {
	daprResp, err := a.daprAPI.GetSecret(ctx, &dapr_v1pb.GetSecretRequest{
		StoreName: in.StoreName,
		Key:       in.Key,
		Metadata:  in.Metadata,
	})
	if err != nil {
		return &runtimev1pb.GetSecretResponse{}, err
	}
	return &runtimev1pb.GetSecretResponse{Data: daprResp.Data}, nil
}

func (a *api) GetBulkSecret(ctx context.Context, in *runtimev1pb.GetBulkSecretRequest) (*runtimev1pb.GetBulkSecretResponse, error) {
	daprResp, err := a.daprAPI.GetBulkSecret(ctx, &dapr_v1pb.GetBulkSecretRequest{
		StoreName: in.StoreName,
		Metadata:  in.Metadata,
	})
	if err != nil {
		return &runtimev1pb.GetBulkSecretResponse{}, err
	}
	return &runtimev1pb.GetBulkSecretResponse{
		Data: convertSecretResponseMap(daprResp.Data),
	}, nil
}

func convertSecretResponseMap(data map[string]*dapr_v1pb.SecretResponse) map[string]*runtimev1pb.SecretResponse {
	if data == nil {
		return nil
	}
	result := make(map[string]*runtimev1pb.SecretResponse)
	for k, v := range data {
		var converted *runtimev1pb.SecretResponse
		if v != nil {
			converted = &runtimev1pb.SecretResponse{
				Secrets: v.Secrets,
			}
		}
		result[k] = converted
	}
	return result
}
