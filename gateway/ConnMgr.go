package gateway

import "github.com/owenliang/go-push/common"

// 推送任务
type PushJob struct {
	pushType int // 推送类型
	roomId string // 房间ID
	// union {
	bizMsg *common.BizMessage 	// 未序列化的业务消息
	wsMsg *common.WSMessage //  已序列化的业务消息
	// }
}

// 连接管理器
type ConnMgr struct {
	buckets []*Bucket
	jobChan []chan*PushJob	 // 每个Bucket对应一个Job Queue

	dispatchChan chan *PushJob	// 待分发消息队列
}

var (
	G_connMgr *ConnMgr
)

// 消息分发到Bucket
func (connMgr *ConnMgr)dispatchWorkerMain(dispatchWorkerIdx int) {
	var (
		bucketIdx int
		pushJob *PushJob
		err error
	)
	for {
		select {
		case pushJob = <- connMgr.dispatchChan: //若待分发消息队列中有消息
			DispatchPending_DESC() //原子操作,记录任务数

			// 序列化
			if pushJob.wsMsg, err = common.EncodeWSMessage(pushJob.bizMsg); err != nil {
				continue //若出现错误则重新执行循环,说明任务失败
			}
			// 分发给所有Bucket, 若Bucket拥塞则等待
			for bucketIdx, _ = range connMgr.buckets {
				PushJobPending_INCR()
				connMgr.jobChan[bucketIdx] <- pushJob //将消息队列直接分发至每一个桶中
			}
		}
	}
}

// Job负责消息广播给客户端
func (connMgr *ConnMgr)jobWorkerMain(jobWorkerIdx int, bucketIdx int) {
	var (
		bucket = connMgr.buckets[bucketIdx]
		pushJob *PushJob
	)

	for {
		select {
		case pushJob = <-connMgr.jobChan[bucketIdx]:	// 从Bucket的job queue取出一个任务
			PushJobPending_DESC()
			if pushJob.pushType == common.PUSH_TYPE_ALL {
				bucket.PushAll(pushJob.wsMsg)
			} else if pushJob.pushType == common.PUSH_TYPE_ROOM {
				bucket.PushRoom(pushJob.roomId, pushJob.wsMsg)
			}
		}
	}
}

/**
	以下是API
 */

func InitConnMgr() (err error) {
	var (
		bucketIdx int
		jobWorkerIdx int
		dispatchWorkerIdx int
		connMgr *ConnMgr
	)

	connMgr = &ConnMgr{
		buckets: make([]*Bucket, G_config.BucketCount),
		jobChan: make([]chan*PushJob, G_config.BucketCount),
		dispatchChan: make(chan*PushJob, G_config.DispatchChannelSize),
	}
	for bucketIdx, _ = range connMgr.buckets {
		connMgr.buckets[bucketIdx] = InitBucket(bucketIdx)	// 初始化Bucket
		connMgr.jobChan[bucketIdx] = make(chan*PushJob, G_config.BucketJobChannelSize) // Bucket的Job队列
		// Bucket的Job worker
		for jobWorkerIdx = 0; jobWorkerIdx < G_config.BucketJobWorkerCount; jobWorkerIdx++ {
			go connMgr.jobWorkerMain(jobWorkerIdx, bucketIdx) //开启协程执行每个桶所含的任务
		}
	}
	// 初始化分发协程, 用于将消息扇出给各个Bucket
	for dispatchWorkerIdx = 0; dispatchWorkerIdx < G_config.DispatchWorkerCount; dispatchWorkerIdx++ {
		go connMgr.dispatchWorkerMain(dispatchWorkerIdx)
	}

	G_connMgr = connMgr
	return
}
//根据连接,取出桶
func (connMgr *ConnMgr) GetBucket(wsConnection *WSConnection) (bucket *Bucket) {
	bucket = connMgr.buckets[wsConnection.connId % uint64(len(connMgr.buckets))]
	return
}
//增加连接
func (connMgr *ConnMgr) AddConn(wsConnection *WSConnection) {
	var (
		bucket *Bucket
	)

	bucket = connMgr.GetBucket(wsConnection)
	bucket.AddConn(wsConnection)

	OnlineConnections_INCR()
}
//删除连接
func (connMgr *ConnMgr) DelConn(wsConnection *WSConnection) {
	var (
		bucket *Bucket
	)

	bucket = connMgr.GetBucket(wsConnection)
	bucket.DelConn(wsConnection)

	OnlineConnections_DESC()
}
//加入房间
func (connMgr *ConnMgr) JoinRoom(roomId string, wsConn *WSConnection) (err error) {
	var (
		bucket *Bucket
	)
	//先获取桶,再加入桶中的房间
	bucket = connMgr.GetBucket(wsConn)
	err = bucket.JoinRoom(roomId, wsConn)
	return
}
//离开房间
func (connMgr *ConnMgr) LeaveRoom(roomId string, wsConn *WSConnection) (err error) {
	var (
		bucket *Bucket
	)

	bucket = connMgr.GetBucket(wsConn)
	err = bucket.LeaveRoom(roomId, wsConn)
	return
}

// 向所有在线用户发送消息
func (connMgr *ConnMgr) PushAll(bizMsg *common.BizMessage) (err error) {
	var (
		pushJob *PushJob
	)

	//增加推送任务
	pushJob = &PushJob{
		pushType: common.PUSH_TYPE_ALL,
		bizMsg: bizMsg,
	}

	select {
	case 	connMgr.dispatchChan <- pushJob:
		DispatchPending_INCR()
	default:
		err = common.ERR_DISPATCH_CHANNEL_FULL
		DispatchFail_INCR()
	}
	return
}

// 向指定房间发送消息
func (connMgr *ConnMgr) PushRoom(roomId string, bizMsg *common.BizMessage) (err error) {
	var (
		pushJob *PushJob
	)

	pushJob = &PushJob{
		pushType: common.PUSH_TYPE_ROOM,
		bizMsg: bizMsg,
		roomId: roomId,
	}

	select {
	case 	connMgr.dispatchChan <- pushJob:
		DispatchPending_INCR()
	default:
		err = common.ERR_DISPATCH_CHANNEL_FULL
		DispatchFail_INCR()
	}
	return
}