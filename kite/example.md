# Json call from browser to kite

On Terminal:


# Some example kite (using dnode) calls

First create a kite instance

    kite = new NewKite({kiteName:"fs-local"})

    kite.tell({method:"fs.readDirectory", withArgs:{path:"/Users/darthvader/Stars", watchSubDirectories:false, onChange:function(){console.log(arguments)}}}, function(){console.log(arguments); console.log("callback is called")})

The message sent over the wire is:

"{"arguments":[{"method":"fs.readDirectory","withArgs":{"path":"/Users/huseyinalb/Documents","watchSubDirectories":false,"onChange":"[Function]"}},"[Function]"],"callbacks":{"0":["0","withArgs","onChange"],"1":["1"]},"method":"fs.readDirectory"}"



