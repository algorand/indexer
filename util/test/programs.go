package test

// BoxApprovalProgram is a TEAL program which allows for testing box functionality
const BoxApprovalProgram string = `#pragma version 8
	txn ApplicationID
	bz end
	txn ApplicationArgs 0   // [arg[0]] // fails if no args && app already exists
	byte "create"			// [arg[0], "create"] // create box named arg[1]
	==                      // [arg[0]=?="create"]
	bz del                  // "create" ? continue : goto del
	int 24                  // [24]
	txn NumAppArgs          // [24, NumAppArgs]
	int 2                   // [24, NumAppArgs, 2]
	==                      // [24, NumAppArgs=?=2]
	bnz default             // WARNING: Assumes that when "create" provided, NumAppArgs >= 3
	pop						// get rid of 24 // NumAppArgs != 2
	txn ApplicationArgs 2   // [arg[2]]         // ERROR when NumAppArgs == 1
	btoi                    // [btoi(arg[2])]
default:                    // [24] // NumAppArgs >= 3
	txn ApplicationArgs 1   // [24, arg[1]]
	swap                   // [arg[1], 24]
	box_create              // [] // boxes: arg[1] -> [24]byte
	assert
	b end
del:						// delete box arg[1]
	txn ApplicationArgs 0   // [arg[0]]
	byte "delete"           // [arg[0], "delete"]
	==                      // [arg[0]=?="delete"]
	bz set                  // "delete" ? continue : goto set
	txn ApplicationArgs 1   // [arg[1]]
	box_del                 // del boxes[arg[1]]
	assert
	b end
set:						// put arg[1] at start of box arg[0] ... so actually a _partial_ "set"
	txn ApplicationArgs 0   // [arg[0]]
	byte "set"              // [arg[0], "set"]
	==                      // [arg[0]=?="set"]
	bz test                 // "set" ? continue : goto test
	txn ApplicationArgs 1   // [arg[1]]
	int 0                   // [arg[1], 0]
	txn ApplicationArgs 2   // [arg[1], 0, arg[2]]
	box_replace             // [] // boxes: arg[1] -> replace(boxes[arg[1]], 0, arg[2])
	b end
test:						// fail unless arg[2] is the prefix of box arg[1]
	txn ApplicationArgs 0   // [arg[0]]
	byte "check"            // [arg[0], "check"]
	==                      // [arg[0]=?="check"]
	bz bad                  // "check" ? continue : goto bad
	txn ApplicationArgs 1   // [arg[1]]
	int 0                   // [arg[1], 0]
	txn ApplicationArgs 2   // [arg[1], 0, arg[2]]
	len                     // [arg[1], 0, len(arg[2])]
	box_extract             // [ boxes[arg[1]][0:len(arg[2])] ]
	txn ApplicationArgs 2   // [ boxes[arg[1]][0:len(arg[2])], arg[2] ]
	==                      // [ boxes[arg[1]][0:len(arg[2])]=?=arg[2] ]
	assert                  // boxes[arg[1]].startwith(arg[2]) ? pop : ERROR
	b end
bad:                        // arg[0] ∉ {"create", "delete", "set", "check"}
	err
end:
	int 1
`

// BoxClearProgram is a vanilla TEAL clear state program
const BoxClearProgram string = `#pragma version 8
int 1
`
