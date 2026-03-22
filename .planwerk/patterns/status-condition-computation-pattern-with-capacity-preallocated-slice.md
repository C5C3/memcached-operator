# Pattern: Status condition computation pattern with capacity-preallocated slice

**Component**: internal/controller
**Category**: service-structure
**Applies-When**: Adding or modifying status conditions in computeConditions()

## Description

All status conditions are computed in computeConditions() as a capacity-preallocated slice (make([]metav1.Condition, 0, N)). Each condition follows the same structure: (1) compute boolean from replica/deployment state, (2) set status/reason pair based on boolean, (3) compute human-readable message with context, (4) append metav1.Condition with Type, Status, Reason, Message, LastTransitionTime, and ObservedGeneration. The capacity N must match the number of conditions. Conditions are then applied to the CR via meta.SetStatusCondition in reconcileStatus().

## Examples

### `internal/controller/status.go:72`

```go
conditions := make([]metav1.Condition, 0, 4)
```

### `internal/controller/status.go:145-167`

```go
ready := desiredReplicas > 0 && readyReplicas == desiredReplicas
readyStatus, readyReason := metav1.ConditionFalse, ConditionReasonNotReady
readyMsg := "Memcached instance is not ready"
if ready {
	readyStatus, readyReason = metav1.ConditionTrue, ConditionReasonReady
	readyMsg = fmt.Sprintf("All %d replicas are ready", desiredReplicas)
}
conditions = append(conditions, metav1.Condition{
	Type:               ConditionTypeReady,
	Status:             readyStatus,
	Reason:             readyReason,
	Message:            readyMsg,
	LastTransitionTime: now,
	ObservedGeneration: mc.Generation,
})
```

