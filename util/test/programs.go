package test

// BoxApprovalProgram is a TEAL program which allows for testing box functionality
const BoxApprovalProgram string = `#pragma version 7
txn ApplicationID
	bz end
	txn ApplicationArgs 0
	byte "create"			// create box named arg[1]
	==
	bz del
	int 24
	txn NumAppArgs
	int 2
	==
	bnz default
	pop						// get rid of 24
	txn ApplicationArgs 2
	btoi
	default:
	txn ApplicationArgs 1
	box_create
	b end
del:						// delete box arg[1]
	txn ApplicationArgs 0
	byte "delete"
	==
	bz set
	txn ApplicationArgs 1
	box_del
	b end
set:						// put arg[1] at start of box arg[0]
	txn ApplicationArgs 0
	byte "set"
	==
	bz test
	txn ApplicationArgs 1
	int 0
	txn ApplicationArgs 2
	box_replace
	b end
test:						// fail unless arg[2] is the prefix of box arg[1]
	txn ApplicationArgs 0
	byte "check"
	==
	bz bad
	txn ApplicationArgs 1
	int 0
	txn ApplicationArgs 2
	len
	box_extract
	txn ApplicationArgs 2
	==
	assert
	b end
bad:
	err
end: 
	int 1`

// BoxClearProgram is a vanilla TEAL clear state program
const BoxClearProgram string = `#pragma version 7
int 1`