from typing import NewType, Optional, List, Any

MessageType = NewType("MessageType", str)

MESSAGE_TYPE_CONNECTION: MessageType = MessageType("CONNECTION")
MESSAGE_TYPE_COMMAND: MessageType = MessageType("COMMAND")
MESSAGE_TYPE_PING: MessageType = MessageType("PING")

class Message:
    """Message class to represent a message sent between the client and server."""
    def __init__(self,
                 type: MessageType,
                 data: Optional[str] = None,
                 arguments: Optional[List[str]] = None,
                 json_data: Optional[Any] = None):
        self.type: MessageType = type
        self.data: Optional[str] = data
        self.arguments: Optional[List[str]] = arguments
        self.json_data: Optional[Any] = json_data

    def to_dict(self) -> dict:
        """Convert the Message object to a dictionary for JSON serialization."""
        return {
            "type": self.type,
            "data": self.data,
            "arguments": self.arguments,
            "json_data": self.json_data,
        }

    def __repr__(self) -> str:
        return (
            f"Message(type={self.type}, "
            f"data={self.data}, "
            f"arguments={self.arguments}, "
            f"json_data={self.json_data})"
        )
