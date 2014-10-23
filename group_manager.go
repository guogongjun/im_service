package main

import "sync"
import "strconv"
import "strings"
import "time"
import "database/sql"
import "github.com/garyburd/redigo/redis"
import _ "github.com/go-sql-driver/mysql"
import log "github.com/golang/glog"

type GroupManager struct {
	mutex sync.Mutex
	groups map[int64]*Group
}

func NewGroupManager() *GroupManager{
    m := new(GroupManager)
    m.groups = make(map[int64]*Group)
    return m
}

func (group_manager *GroupManager) FindGroup(gid int64) *Group {
	group_manager.mutex.Lock()
    defer group_manager.mutex.Unlock()
    if group, ok := group_manager.groups[gid]; ok {
        return group
    }
    return nil
}

func (group_manager *GroupManager) HandleCreate(data string) {
    gid, err := strconv.ParseInt(data, 10, 64)
    if err != nil {
        log.Info("error:", err)
        return
    }

    group_manager.mutex.Lock()
    defer group_manager.mutex.Unlock()

    if _, ok := group_manager.groups[gid]; ok {
        log.Infof("group:%d exists\n", gid)
    }
    log.Info("create group:", gid)
    group_manager.groups[gid] = NewGroup(gid, nil)
}

func (group_manager *GroupManager) HandleDisband(data string) {
    gid, err := strconv.ParseInt(data, 10, 64)
    if err != nil {
        log.Info("error:", err)
        return
    }

    group_manager.mutex.Lock()
    defer group_manager.mutex.Unlock()
    if _, ok := group_manager.groups[gid]; ok {
        log.Info("disband group:", gid)
        delete(group_manager.groups, gid)
    } else {
        log.Infof("group:%d nonexists\n", gid)
    }
}

func (group_manager *GroupManager) HandleMemberAdd(data string) {
    arr := strings.Split(data, ",")
    if len(arr) != 2 {
        log.Info("message error")
        return
    }
    gid, err := strconv.ParseInt(arr[0], 10, 64)
    if err != nil {
        log.Info("error:", err)
        return
    }
    uid, err := strconv.ParseInt(arr[1], 10, 64)
    if err != nil {
        log.Info("error:", err)
        return
    }

    group := group_manager.FindGroup(gid)
    if group != nil {
        group.AddMember(uid)
    } else {
        log.Infof("can't find group:%d\n", gid)
    }
}

func (group_manager *GroupManager) HandleMemberRemove(data string) {
    arr := strings.Split(data, ",")
    if len(arr) != 2 {
        log.Info("message error")
        return
    }
    gid, err := strconv.ParseInt(arr[0], 10, 64)
    if err != nil {
        log.Info("error:", err)
        return
    }
    uid, err := strconv.ParseInt(arr[1], 10, 64)
    if err != nil {
        log.Info("error:", err)
        return
    }

    group := group_manager.FindGroup(gid)
    if group != nil {
        group.RemoveMember(uid)
    } else {
        log.Infof("can't find group:%d\n", gid)
    }
}

func (group_manager *GroupManager) Reload() {
    db, err := sql.Open("mysql", config.mysqldb_datasource)
    if err != nil {
        log.Info("error:", err)
        return
    }
    defer db.Close()

    groups, err := LoadAllGroup(db)
    if err != nil {
        log.Info("error:", err)
        return
    }

    group_manager.mutex.Lock()
    defer group_manager.mutex.Unlock()
    group_manager.groups = groups
}

func (group_manager *GroupManager) RunOnce() bool{
    c, err := redis.Dial("tcp", config.redis_address)
    if err != nil {
        log.Info("dial redis error:", err)
        return false
    }
    psc := redis.PubSubConn{c}
    psc.Subscribe("group_create", "group_disband", "group_member_add", "group_member_remove")
    group_manager.Reload()
    for {
        switch v := psc.Receive().(type) {
        case redis.Message:
            if v.Channel == "group_create" {
                group_manager.HandleCreate(string(v.Data))
            } else if v.Channel == "group_disband" {
                group_manager.HandleDisband(string(v.Data))
            } else if v.Channel == "group_member_add" {
                group_manager.HandleMemberAdd(string(v.Data))
            } else if v.Channel == "group_member_remove" {
                group_manager.HandleMemberRemove(string(v.Data))
            } else {
                log.Infof("%s: message: %s\n", v.Channel, v.Data)
            }
        case redis.Subscription:
            log.Infof("%s: %s %d\n", v.Channel, v.Kind, v.Count)
        case error:
            log.Info("error:", v)
            return true
        }
    }
}

func (group_manager *GroupManager) Run() {
    nsleep := 1
    for {
        connected := group_manager.RunOnce()
        if !connected {
            nsleep *= 2
            if nsleep > 60 {
                nsleep = 60
            }
        } else {
            nsleep= 1
        }
        time.Sleep(time.Duration(nsleep)*time.Second)
    }
}

func (group_manager *GroupManager) Start() {
    go group_manager.Run()
}
