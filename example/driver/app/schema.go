package app

var Schema = `({
	"driver": {
		"properties": {
			"settings": {
				"title": "实例配置",
				"type": "object",
				"properties": {
					"server": {
						"type": "string",
						"title": "服务器",
						"description": "MQTT 服务器地址. 例如: tcp://127.0.0.1:1883"
					},
					"username": {
						"type": "string",
						"title": "用户名"
					},
					"password": {
						"type": "string",
						"title": "密码",
						"fieldType": "password"
					},
					"clientId": {
						"type": "string",
						"title": "客户端ID"
					},
					"topic": {
						"type": "string",
						"title": "主题",
						"description": "接收数据的主题. 例如: /data/#"
					},
					"parseScript": {
						"type": "string",
						"title": "数据处理脚本",
						"fieldType": "deviceScriptEdit",
						"description": "消息处理脚本. 函数名必须为 'handler'",
						"defaultScript": "/**\n" +
							" * 数据处理脚本, 处理从 mqtt 接收到的数据.\n" +
							" *\n" +
							" * @param {string} topic 消息主题\n" +
							" * @param {string} message 消息内容\n" +
							" * @return 消息解析结果\n" +
							" */\n" +
							"function handler(topic, message) {\n" +
							"\t\n" +
							"\t// 脚本返回值必须为对象数组\n" +
							"\t// \tid: 设备编号\n" +
							"\t//\ttime: 时间戳(毫秒)\n" +
							"\t//  fields: 数据点数据. 该字段为 JSON 对象, key 为数据点标识, value 为数据点的值\n" +
							"\treturn [\n" +
							"\t\t{\"table\": \"T10001\", \"id\": \"SN10001\", \"time\": new Date().getTime(), \"fields\": {\"key1\": \"this is a string value\", \"key2\": true, \"key3\": 123.456}}\n" +
							"\t];\n" +
							"}"
					},
					"commandScript": {
						"type": "string",
						"title": "指令处理脚本",
						"fieldType": "deviceScriptEdit",
						"description": "指令处理脚本. 函数名必须为 'handler'",
						"defaultScript": "/**\n" +
							" * 指令处理脚本. 发送指令时会将指令内容传递给脚本, 然后由指定返回最终要发送的信息.\n" +
							" *\n" +
							" * @param {string} 工作表标识\n" +
							" * @param {string} 设备编号\n" +
							" * @param {object} 命令内容\n" +
							" * @return {object} 最终要发送的消息, 及目标 topic\n" +
							" */\n" +
							"function handler(tableId, deviceId, command) {\n" +
							"\t\n" +
							"\t// 脚本返回值必须为下面对象结构\n" +
							"\t//\t\ttopic: 消息发送的目标 topic\n" +
							"\t//\t\tpayload: 消息内容\n" +
							"\treturn {\n" +
							"\t\t\"topic\": \"cmd/\" + deviceId,\n" +
							"\t\t\"payload\": \"发送内容\"\n" +
							"\t};\n" +
							"}"
					},
					"network": {
						"type": "object",
						"title": "通讯监控参数",
						"properties": {
							"timeout": {
								"title": "通讯超时时间(s)",
								"description": "经过多长时间仪表还没有任何数据上传，认定为通讯故障",
								"type": "number"
							}
						}
					}
				},
				"required": ["server", "username", "password", "topic", "parseScript", "commandScript"]
			}
		}
	},
	"model": {
		"properties": {
			"settings": {
				"title": "模型配置",
				"type": "object",
				"properties": {
					"network": {
						"type": "object",
						"title": "通讯监控参数",
						"properties": {
							"timeout": {
								"title": "通讯超时时间(s)",
								"description": "经过多长时间仪表还没有任何数据上传，认定为通讯故障",
								"type": "number"
							}
						},
						"required": []
					}
				},
				"required": []
			},
			"tags": {
				"title": "数据点",
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"id": {
							"type": "string",
							"title": "标识",
							"description": "数据点的标识, 用于在数据点列表中唯一标识数据点"
						},
						"name": {
							"type": "string",
							"title": "名称",
							"description": "数据点的名称"
						}
					},
					"required": ["id", "name"]
				}
			},
			"commands": {
				"title": "命令",
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"name": {
							"type": "string",
							"title": "名称"
						},
"commandType": {
										"type": "string",
										"title": "指令类型",
										"enum": ["delay", "common"],
										"enum_title": ["延时指令", "普通指令"]
									},
									"commandDelay": {
										"type": "object",
										"title": "延时配置",
										"properties": {
											"enable": {
												"type": "boolean",
												"title": "启用"
											},
											"timeout": {
												"type": "number",
												"title": "超时"
											}
										},
										"required": []
									},
						"ops": {
							"type": "array",
							"title": "指令",
							"items": {
								"type": "object",
								"properties": {
									"topic": {
										"type": "string",
										"title": "主题",
										"description": "发送消息的主题. 例如: /cmd/control"
									},
									"message": {
										"type": "string",
										"title": "消息",
										"description": "发送的消息. 例如: {\"cmd\":\"start\"}"
									},
									"qos": {
										"type": "number",
										"title": "QoS",
										"description": "消息质量. 0,1,2",
										"enum": [0, 1, 2],
										"enum_title": ["QoS0", "QoS1", "QoS2"]
									}
								},
								"required": ["name", "message"]
							}
						}
					},
					"required": []
				}
			}
		},
		"required": []
	},
	"device": {
		"properties": {
			"settings": {
				"title": "设备配置",
				"type": "object",
				"properties": {
					"customDeviceId": {
						"type": "string",
						"title": "设备编号",
						"descripption": "自定义设备编号. 如果未定义则使用平台中的设备编号"
					},
					"network": {
						"type": "object",
						"title": "通讯监控参数",
						"properties": {
							"timeout": {
								"title": "通讯超时时间(s)",
								"description": "经过多长时间仪表还没有任何数据上传，认定为通讯故障",
								"type": "number"
							}
						}
					}
				},
				"required": []
			}
		}
	}
})`
