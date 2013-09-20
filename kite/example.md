# Json call from browser to kite

On Terminal:

1. go run mathworker.go

On Browser

1. url = "ws://127.0.0.1:4000/sock";ws = new WebSocket(url);
2. ws.onmessage = function(data) {return console.log("rpc result:", data.data);};
3. ws.send(JSON.stringify({method: "mathworker.WsSquare", params: [5] }));


# Dnode call


kite = new NewKite({kiteName:"fs-local"})
kite.tell({method:"fs.readDirectory", withArgs:{path:"/Users/huseyinalb/Documents", watchSubDirectories:false, onChange:function(){console.log(arguments)}}}, function(err, hede){console.log(arguments); console.log("saddassad")})


scrubbed dnode message sent over the wire:
"{"arguments":[{"method":"fs.readDirectory","withArgs":{"path":"/Users/huseyinalb/Documents","watchSubDirectories":false,"onChange":"[Function]"}},"[Function]"],"callbacks":{"0":["0","withArgs","onChange"],"1":["1"]},"method":"fs.readDirectory"}"