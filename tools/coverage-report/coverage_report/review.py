"""Reviewer identity validation.

A mapping file's ``reviewed_by`` field decides whether its entries count as
declared evidence. The check must fail closed: anything that is not clearly a
GitHub user or team handle is unreviewed. A placeholder is a deliberate "not
reviewed yet" marker, not an invalid value.
"""

import re

#: A GitHub user handle (``@name`` optional) or an organization/team pair.
REVIEWER_PATTERN = re.compile(
    r"^@?[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})"
    r"(?:/[A-Za-z0-9][A-Za-z0-9._-]*)?$"
)

#: Case-insensitive markers that mean "nobody has reviewed this yet".
PLACEHOLDERS = frozenset(
    {"pending-review", "pending", "todo", "tbd", "unreviewed", "none", "n/a", "na"}
)


def is_placeholder(value: object) -> bool:
    """Return whether the value is a recognized not-reviewed marker."""
    return isinstance(value, str) and value.strip().casefold() in PLACEHOLDERS


def is_reviewed(value: object) -> bool:
    """Return whether ``value`` names a real reviewer.

    Only a non-empty string matching :data:`REVIEWER_PATTERN` that is not a
    placeholder counts. Lists, booleans, numbers, empty strings, and prose are
    all unreviewed.
    """
    if not isinstance(value, str):
        return False
    text = value.strip()
    if not text or is_placeholder(text):
        return False
    return REVIEWER_PATTERN.match(text) is not None


def is_invalid_reviewer(value: object) -> bool:
    """Return whether a present value was meant as a reviewer but cannot be one.

    ``None`` (field absent) and recognized placeholders are not invalid; a list,
    a boolean, or malformed prose is.
    """
    if value is None or is_placeholder(value):
        return False
    if not isinstance(value, str):
        return True
    text = value.strip()
    if not text:
        return False
    return REVIEWER_PATTERN.match(text) is None
