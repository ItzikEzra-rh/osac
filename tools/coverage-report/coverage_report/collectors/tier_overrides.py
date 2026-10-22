"""Per-behavior tier assignments from ``config/mappings/<KEY>.yaml``.

A test plan is the primary source of a behavior's tier. When it does not
declare one, a QE reviewer may add a ``behavior_tiers`` section to the feature's
mapping file:

.. code-block:: yaml

    behavior_tiers:
      TC-FR1-01: integration

The override wins over the test plan so a reviewer can correct it without
editing the design document. Overrides in a file whose ``reviewed_by`` is still
``PENDING-REVIEW`` still apply, but the behavior records ``tier_reviewed:
false`` so the report can flag the assignment.

Tier assignment never invents a tier: a behavior with no test-plan tier and no
override stays unassigned, which blocks the tier metrics instead of producing a
precise-looking percentage from an incomplete denominator.
"""

import os
from dataclasses import dataclass
from typing import Dict, List, Optional

import yaml

from ..config import bundled_features_file
from ..manifest import relative_path
from ..model import Diagnostic, Tier
from ..review import is_reviewed
from . import CollectorContext

ORIGIN_TESTPLAN = "testplan"
ORIGIN_MAPPING_CONFIG = "mapping-config"

OPERATION = "resolve_tier"

CONFIG_DIR_NAME = "mappings"


@dataclass(frozen=True)
class TierAssignment:
    """One behavior's resolved tier and where it came from."""

    tier: Tier
    origin: str
    reviewed: bool


def mapping_config_path(ctx: CollectorContext, feature_key: str) -> Optional[str]:
    """Return the mapping file for a feature, repository first, then bundled."""
    directories = (
        os.path.join(ctx.repo_root, "config", CONFIG_DIR_NAME),
        os.path.join(os.path.dirname(bundled_features_file()), CONFIG_DIR_NAME),
    )
    # .yaml wins over .yml in each location; the repository wins over the
    # bundled copy. This is the single place the convention is decided.
    for directory in directories:
        for extension in (".yaml", ".yml"):
            candidate = os.path.join(directory, feature_key + extension)
            if os.path.isfile(candidate):
                return candidate
    return None


def _diagnostic(
    ctx: CollectorContext,
    *,
    code: str,
    message: str,
    severity: str,
    recovery: str = "",
) -> Diagnostic:
    return Diagnostic(
        run_id=ctx.run_id,
        collector="behaviors",
        operation=OPERATION,
        code=code,
        message=message,
        severity=severity,
        recovery=recovery,
    )


def load_tier_overrides(
    ctx: CollectorContext,
    feature,
    *,
    diagnostics: List[Diagnostic],
) -> Dict[str, TierAssignment]:
    """Return ``behavior id -> TierAssignment`` from the mapping file.

    Recoverable problems become diagnostics; the returned map only contains
    entries that name a known tier. Unknown behavior ids are reported so a typo
    cannot silently leave a behavior unassigned.
    """
    path = mapping_config_path(ctx, feature.key)
    if path is None:
        return {}

    display = relative_path(ctx.repo_root, path)
    try:
        with open(path, "r", encoding="utf-8") as handle:
            data = yaml.safe_load(handle)
    except (OSError, yaml.YAMLError):
        # The mapping collector owns mapping-file diagnostics; a tier reader
        # must not fail the run over a file it does not own.
        return {}

    if not isinstance(data, dict):
        return {}
    raw = data.get("behavior_tiers")
    if raw is None:
        return {}
    if not isinstance(raw, dict):
        diagnostics.append(
            _diagnostic(
                ctx,
                code="TIER_OVERRIDE_INVALID",
                message="{0} behavior_tiers must be a mapping of test case id "
                "to tier".format(display),
                severity="warning",
                recovery="Use behavior_tiers: {TC-FR1-01: integration}.",
            )
        )
        return {}

    reviewed = is_reviewed(data.get("reviewed_by"))
    assignments: Dict[str, TierAssignment] = {}
    for raw_id in sorted(raw):
        if not isinstance(raw_id, str) or not raw_id.strip():
            diagnostics.append(
                _diagnostic(
                    ctx,
                    code="TIER_OVERRIDE_INVALID",
                    message="{0} behavior_tiers has a non-string key".format(display),
                    severity="warning",
                    recovery="Use test case ids such as TC-FR1-01.",
                )
            )
            continue
        behavior_id = raw_id.strip()
        if "/" not in behavior_id:
            behavior_id = "{0}/{1}".format(feature.key, behavior_id)

        value = raw[raw_id]
        try:
            tier = Tier(str(value).strip().lower())
        except (TypeError, ValueError):
            diagnostics.append(
                _diagnostic(
                    ctx,
                    code="TIER_OVERRIDE_INVALID",
                    message="{0} assigns tier {1!r} to {2}; expected unit, "
                    "integration, or e2e".format(display, value, raw_id),
                    severity="warning",
                    recovery="Use one of: unit, integration, e2e.",
                )
            )
            continue

        assignments[behavior_id] = TierAssignment(
            tier=tier,
            origin=ORIGIN_MAPPING_CONFIG,
            reviewed=reviewed,
        )
    return assignments


def report_unknown_overrides(
    ctx: CollectorContext,
    assignments: Dict[str, TierAssignment],
    known_ids: set,
    diagnostics: List[Diagnostic],
) -> None:
    """Report overrides that do not name a behavior in the inventory."""
    for behavior_id in sorted(set(assignments) - known_ids):
        diagnostics.append(
            _diagnostic(
                ctx,
                code="TIER_OVERRIDE_UNKNOWN_BEHAVIOR",
                message="behavior_tiers names {0}, which is not in the "
                "inventory".format(behavior_id),
                severity="warning",
                recovery="Use a TC id from the feature test plan.",
            )
        )
