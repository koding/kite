# Json call from browser to kite

On Terminal:


# Some example kite (using dnode) calls

First create a kite instance

    kite = new NewKite({kitename:"fs"})

    kite.tell({method:"fs.readDirectory", withArgs:{path:"/Users/darthvader/Stars", watchSubDirectories:false, onChange:function(){console.log(arguments)}}}, function(){console.log(arguments); console.log("callback is called")})

The message sent over the wire is:

	"{"arguments":[{"method":"fs.readDirectory","withArgs":{"path":"/Users/huseyinalb/Documents","watchSubDirectories":false,"onChange":"[Function]"}},"[Function]"],"callbacks":{"0":["0","withArgs","onChange"],"1":["1"]},"method":"fs.readDirectory"}"

# example usage for s3 kite

	kite = new NewKite({kitename:"s3"})
	// store a file
	kite.tell({method:"s3.store", withArgs:{name:"hede", content:"aGVsbG8gd29ybGQ="}}, function(err, res){console.log(err, res); console.log("Done")})
	// see the file, clicking the link given by s3.get
	FSHelper.s3.get("hede")
	// delete it, and click the link again to see "access denied"
	kite.tell({method:"s3.delete", withArgs:{name:"hede"}}, function(err, res){console.log(err, res); console.log("Done")})

# example usage for fs kite

