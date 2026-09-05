package app

import "context"

type guestListOperationInput struct {
	EventID    string
	Collection collectionOptions
}

type blastSendOperationInput struct {
	EventID         string
	Audience        string
	ShowOnEventPage bool
	Message         blastPreparedMessage
}

type productInvocation struct {
	definition   commandDefinition
	execution    mutationExecution
	collection   collectionOptions
	eventID      string
	guestList    guestListOperationInput
	guestInvite  guestInviteOptions
	eventCreate  eventCreateOptions
	eventUpdate  eventUpdateOptions
	eventCancel  eventCancelOptions
	blastSend    blastSendOperationInput
	rsvpSet      rsvpSetOptions
	cohostAction cohostActionOptions
	cohostLink   cohostLinkOptions
}

func parseCLIProductInvocation(
	request Request,
	definition commandDefinition,
	argv []string,
	dependencies Dependencies,
	execution mutationExecution,
) (productInvocation, *errorBody) {
	invocation := productInvocation{definition: definition, execution: execution}
	switch definition.kind {
	case postersListCommand, postersSearchCommand, contactsListCommand, eventsListCommand:
		options, inputError := parseCollectionOptions(definition, argv)
		invocation.collection = options
		return invocation, inputError
	case guestsListCommand:
		eventID, options, inputError := parseGuestListOptions(definition, argv)
		invocation.guestList = guestListOperationInput{EventID: eventID, Collection: options}
		return invocation, inputError
	case guestsInviteCommand:
		options, inputError := parseGuestInviteOptions(definition, argv)
		invocation.guestInvite = options
		return invocation, inputError
	case eventsGetCommand, rsvpGetCommand:
		eventID, inputError := parseEventID(definition, argv)
		invocation.eventID = eventID
		return invocation, inputError
	case eventsCreateCommand:
		options, inputError := parseEventCreateOptions(request, definition, argv, dependencies)
		invocation.eventCreate = options
		return invocation, inputError
	case eventsUpdateCommand:
		options, inputError := parseEventUpdateOptions(request, definition, argv, dependencies)
		invocation.eventUpdate = options
		return invocation, inputError
	case eventsCancelCommand:
		options, inputError := parseEventCancelOptions(request, definition, argv, dependencies)
		invocation.eventCancel = options
		return invocation, inputError
	case blastsSendCommand:
		options, inputError := parseBlastSendOptions(definition, argv)
		if inputError != nil {
			return productInvocation{}, inputError
		}
		message, inputError := readBlastMessage(request, dependencies, options.MessageFile)
		invocation.blastSend = blastSendOperationInput{
			EventID:         options.EventID,
			Audience:        options.Audience,
			ShowOnEventPage: options.ShowOnEventPage,
			Message:         message,
		}
		return invocation, inputError
	case rsvpSetCommand:
		options, inputError := parseRSVPSetOptions(request, definition, argv, dependencies)
		invocation.rsvpSet = options
		return invocation, inputError
	case cohostsInviteCommand, cohostsRevokeInviteCommand, cohostsRemoveCommand:
		options, inputError := parseCohostActionOptions(definition, argv)
		invocation.cohostAction = options
		return invocation, inputError
	case cohostsLinkCreateCommand, cohostsLinkRevokeCommand:
		options, inputError := parseCohostLinkOptions(definition, argv)
		invocation.cohostLink = options
		return invocation, inputError
	default:
		return productInvocation{}, eventWriteInputFailure("COMMAND_NOT_INVOCABLE", "The command is not a product operation.")
	}
}

func invokeProductOperation(
	ctx context.Context,
	invocation productInvocation,
	dependencies Dependencies,
	pretty bool,
) Result {
	definition := invocation.definition
	switch definition.kind {
	case postersListCommand, postersSearchCommand:
		return executePosters(ctx, definition, invocation.collection, dependencies, pretty)
	case contactsListCommand:
		return executeContacts(ctx, definition, invocation.collection, dependencies, pretty)
	case guestsListCommand:
		return executeGuestsList(ctx, definition, invocation.guestList, dependencies, pretty)
	case guestsInviteCommand:
		return executeGuestsInvite(ctx, definition, invocation.guestInvite, dependencies, invocation.execution, pretty)
	case eventsListCommand:
		return executeEventsList(ctx, definition, invocation.collection, dependencies, pretty)
	case eventsGetCommand:
		return executeEventGet(ctx, definition, invocation.eventID, dependencies, pretty)
	case eventsCreateCommand:
		return executeEventCreate(ctx, definition, invocation.eventCreate, dependencies, invocation.execution, pretty)
	case eventsUpdateCommand:
		return executeEventUpdate(ctx, definition, invocation.eventUpdate, dependencies, invocation.execution, pretty)
	case eventsCancelCommand:
		return executeEventCancel(ctx, definition, invocation.eventCancel, dependencies, invocation.execution, pretty)
	case blastsSendCommand:
		return executeBlastSend(ctx, definition, invocation.blastSend, dependencies, invocation.execution, pretty)
	case rsvpGetCommand:
		return executeRSVPGet(ctx, definition, invocation.eventID, dependencies, pretty)
	case rsvpSetCommand:
		return executeRSVPSet(ctx, definition, invocation.rsvpSet, dependencies, invocation.execution, pretty)
	case cohostsInviteCommand:
		return executeCohostInvite(ctx, definition, invocation.cohostAction, dependencies, invocation.execution, pretty)
	case cohostsRevokeInviteCommand:
		return executeCohostRevokeInvite(ctx, definition, invocation.cohostAction, dependencies, invocation.execution, pretty)
	case cohostsRemoveCommand:
		return executeCohostRemove(ctx, definition, invocation.cohostAction, dependencies, invocation.execution, pretty)
	case cohostsLinkCreateCommand:
		return executeCohostLinkCreate(ctx, definition, invocation.cohostLink, dependencies, invocation.execution, pretty)
	case cohostsLinkRevokeCommand:
		return executeCohostLinkRevoke(ctx, definition, invocation.cohostLink, dependencies, invocation.execution, pretty)
	default:
		return internalFailure(definition.path, pretty)
	}
}
