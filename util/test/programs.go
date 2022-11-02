package test

import _ "embed" // embed teal programs as string variables

// BoxApprovalProgram is a TEAL program which allows for testing box functionality
//
//go:embed boxes.teal
var BoxApprovalProgram string

// BoxClearProgram is a vanilla TEAL clear state program
const BoxClearProgram string = `#pragma version 8
int 1
`
