# Json call from browser to kite

On Terminal:

1. go run mathworker.go

On Browser

1. url = "ws://127.0.0.1:4000/sock";ws = new WebSocket(url);
2. ws.onmessage = function(data) {return console.log("rpc result:", data.data);};
3. ws.send(JSON.stringify({method: "mathworker.WsSquare", params: [5] }));
