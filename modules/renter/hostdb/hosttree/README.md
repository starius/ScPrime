# Host Tree
Coming Soon...
<<<<<<< HEAD
=======

## Weight Function
The HostTree Weight Function is a function used to weight a given HostDBEntry in
the tree. Each HostTree is initialized with a weight function. This allows the
entries to be selected at random, weighted by the hosttree's weight function.

**NOTE:** Developers should be aware of where the weight function is defined and
what it uses to determine the weight of an entry. The weight function may or may
not require a lock from another package in order to calculate the weight safely.
>>>>>>> 7a752c5725cecd036380608233b7c116fcd37561
