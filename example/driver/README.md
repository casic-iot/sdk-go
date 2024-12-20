# sdk-go-demo

驱动开发包例子

## 测试数据

topic: test/gosdk/gosdk1

```json
[
  {
    "key": "p1",
    "value": 1
  },
  {
    "key": "p2",
    "value": 2
  }
]
```

## 解析脚本

```javascripta
// 脚本返回值必须为对象数组
// id: 设备编号
// time: 时间戳(毫秒)
// fields: 数据点数据. 该字段为 JSON 对象, key 为数据点标识, value 为数据点的值
function handler(topic, message) {
  console.log("handler message", message)
  try {
    let arr = message
    console.log("handler arr", arr)
    let topics = topic.split("/");
    let field = {}
    arr.forEach(ele => {
      field[ele.key] = ele.value
    })
    return [
      {"table": topics[1], "id": topics[2], "time": new Date().getTime(), "fields": field}
    ]
  } catch (e) {
    console.error("handler error", e)
    return []
  }
}
```

## 指令脚本

topic: cmd/#

```javascripta
function handler(tableId, deviceId, command) {
    return {"topic": "cmd/" + tableId + "/" + deviceId, "payload": JSON.stringify(command.params)}
  }
```