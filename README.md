# Home

# How to run

1. put config in config.yaml from config.yaml.sample
2. go run . run

# Config applied

https://tasmota.github.io/docs/MQTT/#command-flow

Show power state on LED (LED on when power on) 
```
LedState 1
```

Disable LED:
```
LedPower 0
```

Set correct timezone:
```
Backlog0 Timezone 99; TimeStd 0,0,10,1,4,120; TimeDst 0,0,3,1,3,180
```

Set correct Voltage
```
VoltageSet 220
```

Allow save config
```
SaveData 1
```

Set Template after weird reset
```
Template {"NAME":"NOUS A1T","GPIO":[32,0,0,0,2720,2656,0,0,2624,320,224,0,0,0],"FLAG":0,"BASE":49}
Module 0
```

Sensor sending frequency
```
TelePeriod 15
```