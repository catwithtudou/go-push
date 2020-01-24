package gateway

import (
	"sync"
	"github.com/owenliang/go-push/common"
)

//长连接分成多个桶
//桶实体
type Bucket struct {
	rwMutex sync.RWMutex
	index int // 我是第几个桶
	id2Conn map[uint64]*WSConnection	// 连接列表(key=连接唯一ID)
	rooms map[string]*Room // 房间列表
}

//初始化桶结构
func InitBucket(bucketIdx int) (bucket *Bucket) {
	bucket = &Bucket{
		index: bucketIdx,
		id2Conn: make(map[uint64]*WSConnection),
		rooms: make(map[string]*Room),
	}
	return
}

//建立连接
func (bucket *Bucket) AddConn(wsConn *WSConnection) {
	//使用互斥锁,保证连接唯一
	bucket.rwMutex.Lock()
	defer bucket.rwMutex.Unlock()

	bucket.id2Conn[wsConn.connId] = wsConn
}

//删除连接
func (bucket *Bucket) DelConn(wsConn *WSConnection) {
	//使用互斥锁,保证连接唯一
	bucket.rwMutex.Lock()
	defer bucket.rwMutex.Unlock()

	delete(bucket.id2Conn, wsConn.connId)
}

//加入房间,即一个房间一个桶
func (bucket *Bucket) JoinRoom(roomId string, wsConn *WSConnection) (err error) {
	var (
		existed bool //是否存在
		room *Room
	)
	//保证加入时唯一
	bucket.rwMutex.Lock()
	defer bucket.rwMutex.Unlock()

	// 找到房间
	if room, existed = bucket.rooms[roomId]; !existed {  //若房间不存在,则建立以房间id作为桶的字典key
		room = InitRoom(roomId)
		bucket.rooms[roomId] = room
		RoomCount_INCR()  //原子性操作,房间数量+1
	}
	// 加入房间
	err = room.Join(wsConn)
	return
}

//离开房间
func (bucket *Bucket) LeaveRoom(roomId string, wsConn *WSConnection) (err error) {
	var (
		existed bool
		room *Room
	)
	bucket.rwMutex.Lock()
	defer bucket.rwMutex.Unlock()

	// 找到房间
	if room, existed = bucket.rooms[roomId]; !existed { //若房间不存在
		err = common.ERR_NOT_IN_ROOM
		return
	}

	err = room.Leave(wsConn)

	// 房间为空, 则删除
	if room.Count() == 0 {
		delete(bucket.rooms, roomId)
		RoomCount_DESC() //原子操作
	}
	return
}

// 推送给Bucket内所有用户
func (bucket *Bucket) PushAll(wsMsg *common.WSMessage) {
	var (
		wsConn *WSConnection
	)

	// 锁Bucket
	bucket.rwMutex.RLock()
	defer bucket.rwMutex.RUnlock()

	// 全量非阻塞推送
	for _, wsConn = range bucket.id2Conn {
		wsConn.SendMessage(wsMsg)
	}
}

// 推送给某个房间的所有用户
func (bucket *Bucket) PushRoom(roomId string, wsMsg *common.WSMessage) {
	var (
		room *Room
		existed bool
	)

	// 锁Bucket
	bucket.rwMutex.RLock()
	room, existed = bucket.rooms[roomId]
	bucket.rwMutex.RUnlock()

	// 房间不存在
	if !existed {
		return
	}

	// 向房间做推送
	room.Push(wsMsg)
}
