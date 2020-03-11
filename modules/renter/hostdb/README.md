# HostDB
Coming Soon...
<<<<<<< HEAD
=======

## Weight Function
The hostdb gets initialized with an allowance that can be modified. The
allowance is used to build a weight function that the hosttree depends on to
determine the weight of a host. Currently, `managedCalculateHostWeightFn` is
used to create a `hosttree.WeightFunc` for the hostdb. The weight function that
is returned accesses fields of the hostdb when called to calculate a weight for
an entry. This means that the hostdb lock must be held when calling the weight
function.
>>>>>>> 7a752c5725cecd036380608233b7c116fcd37561
