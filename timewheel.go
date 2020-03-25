package timewheel

import (
	"container/list"
	"fmt"
	"time"
)

// 时间轮结构体
type TimeWheel struct {
	interval          time.Duration       //槽位时间单位
	ticker            *time.Ticker        //
	slots             []*list.List        //时间轮盘
	timer             map[interface{}]int //任务位置记录器
	currentPos        int                 //当前指针位置
	slotNum           int                 //轮盘数量
	addTaskChannel    chan task           //新增任务channel
	updateTaskChannel chan uptask         //更新任务channel
	removeTaskChannel chan interface{}    //删除任务channel
	stopChannel       chan bool           //停止定时器channel
}

// Job 任务回调函数
type Job func(TaskData)

// TaskData 回调函数参数
type TaskData map[interface{}]interface{}

// task 任务结构体
type task struct {
	interval time.Duration //时间间隔
	times    int           //-1:无限次 >=1:指定运行次数
	circle   int           //时间轮圈数
	key      interface{}   //定时器唯一标识
	job      Job           //回调函数
	taskData TaskData      //回调函数参数
}

// uptask 结构体
type uptask struct {
	key      interface{}   //定时器唯一标识
	interval time.Duration //时间间隔
	taskData TaskData      //回调函数参数
}

// New 创建时间轮
func New(interval time.Duration, slotNum int) *TimeWheel {
	if interval <= 0 || slotNum <= 0 {
		return nil
	}
	tw := &TimeWheel{
		interval:          interval,
		slots:             make([]*list.List, slotNum),
		timer:             make(map[interface{}]int),
		currentPos:        0,
		slotNum:           slotNum,
		addTaskChannel:    make(chan task),
		updateTaskChannel: make(chan uptask),
		removeTaskChannel: make(chan interface{}),
		stopChannel:       make(chan bool),
	}

	tw.init()

	return tw
}

// Start 启动时间轮
func (tw *TimeWheel) Start() {
	tw.ticker = time.NewTicker(tw.interval)
	go tw.start()
}

// Stop 停止时间轮
func (tw *TimeWheel) Stop() {
	tw.stopChannel <- true
}

func (tw *TimeWheel) start() {
	for {
		select {
		case <-tw.ticker.C:
			tw.tickHandler()
		case task := <-tw.addTaskChannel:
			tw.addTask(&task)
		case update := <-tw.updateTaskChannel:
			tw.updateTask(update)
		case key := <-tw.removeTaskChannel:
			tw.removeTask(key)
		case <-tw.stopChannel:
			tw.ticker.Stop()
			return
		}
	}
}

func (tw *TimeWheel) AddTask(interval time.Duration, times int, key interface{}, data TaskData, job Job) {
	if interval <= 0 || key == nil || job == nil {
		return
	}
	tw.addTaskChannel <- task{interval: interval, times: times, key: key, taskData: data, job: job}
}

func (tw *TimeWheel) UpdateTask(key interface{}, interval time.Duration, data TaskData) {
	if key == nil {
		return
	}
	tw.updateTaskChannel <- uptask{key: key, interval: interval, taskData: data}
}

func (tw *TimeWheel) RemoveTask(key interface{}) {
	if key == nil {
		return
	}
	tw.removeTaskChannel <- key
}

// 时间轮初始化
func (tw *TimeWheel) init() {
	for i := 0; i < tw.slotNum; i++ {
		tw.slots[i] = list.New()
	}
}

//
func (tw *TimeWheel) tickHandler() {
	l := tw.slots[tw.currentPos]
	tw.scanAddRunTask(l)
	if tw.currentPos == tw.slotNum-1 {
		tw.currentPos = 0
	} else {
		tw.currentPos++
	}
}

// 添加任务
func (tw *TimeWheel) addTask(task *task) {
	pos, circle := tw.getPositionAndCircle(task.interval)
	task.circle = circle

	tw.slots[pos].PushBack(task)

	tw.timer[task.key] = pos
}

// 更新任务参数
func (tw *TimeWheel) updateTask(update uptask) {
	// 获取定时器所在的槽
	key := update.key
	position, ok := tw.timer[key]
	if !ok {
		return
	}
	fmt.Println(update)
	// 获取槽指向的链表
	l := *tw.slots[position]
	for e := l.Front(); e != nil; {
		task := e.Value.(*task)
		if task.key == key { //修改
			task.interval = update.interval
			task.taskData = update.taskData
		}

		e = e.Next()
	}
}

// 移除任务
func (tw *TimeWheel) removeTask(key interface{}) {
	// 获取定时器所在的槽
	position, ok := tw.timer[key]
	if !ok {
		return
	}
	// 获取槽指向的链表
	l := tw.slots[position]
	for e := l.Front(); e != nil; {
		task := e.Value.(*task)
		if task.key == key {
			delete(tw.timer, task.key)
			l.Remove(e)
		}

		e = e.Next()
	}
}

// 扫描链表中任务并执行回调函数
func (tw *TimeWheel) scanAddRunTask(l *list.List) {

	if l == nil {
		return
	}

	for item := l.Front(); item != nil; {
		task := item.Value.(*task)

		if task.times == 0 {
			next := item.Next()
			l.Remove(item)
			delete(tw.timer, task.key)
			item = next
			continue
		}

		if task.circle > 0 {
			task.circle--
			item = item.Next()
			continue
		}

		go task.job(task.taskData)
		next := item.Next()
		l.Remove(item)
		delete(tw.timer, task.key)
		item = next

		//周期任务重新添加到轮盘
		if task.times > 0 || task.times == -1 {
			if task.times > 0 {
				task.times--
			}
			tw.addTask(task)
		}
	}
}

// 获取定时器在槽中的位置, 时间轮需要转动的圈数
func (tw *TimeWheel) getPositionAndCircle(d time.Duration) (pos int, circle int) {
	delaySeconds := int(d.Seconds())
	intervalSeconds := int(tw.interval.Seconds())
	circle = int(delaySeconds / intervalSeconds / tw.slotNum)
	pos = int(tw.currentPos+delaySeconds/intervalSeconds) % tw.slotNum
	return
}
