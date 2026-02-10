package app

import "fmt"

func wrapSessionCommand(command, runID string) string {
	return fmt.Sprintf("{ __lisa_run_id=%s; __lisa_hb_pid=''; __lisa_ec=0; __lisa_marker_done=0; __lisa_hb_tick(){ if [ -n \"${LISA_HEARTBEAT_FILE:-}\" ]; then : > \"$LISA_HEARTBEAT_FILE\" 2>/dev/null || true; fi; }; __lisa_hb_start(){ if [ -n \"${LISA_HEARTBEAT_FILE:-}\" ]; then __lisa_hb_tick; (while :; do __lisa_hb_tick; sleep 2; done) & __lisa_hb_pid=$!; fi; }; __lisa_hb_stop(){ if [ -n \"$__lisa_hb_pid\" ]; then kill \"$__lisa_hb_pid\" >/dev/null 2>&1 || true; wait \"$__lisa_hb_pid\" 2>/dev/null || true; __lisa_hb_pid=''; fi; __lisa_hb_tick; }; __lisa_emit_done(){ if [ \"$__lisa_marker_done\" -eq 0 ]; then printf '\\n%s%%s:%%d\\n' \"$__lisa_run_id\" \"$__lisa_ec\"; __lisa_marker_done=1; fi; }; __lisa_cleanup(){ __lisa_hb_stop; __lisa_emit_done; }; trap '__lisa_ec=130; exit \"$__lisa_ec\"' INT TERM HUP; trap '__lisa_cleanup' EXIT; __lisa_hb_start; printf '\\n%s%%s:%%s\\n' \"$__lisa_run_id\" \"$(date +%%s)\"; __lisa_had_errexit=0; case $- in *e*) __lisa_had_errexit=1;; esac; set +e; %s; __lisa_ec=$?; if [ \"$__lisa_had_errexit\" -eq 1 ]; then set -e; fi; exit \"$__lisa_ec\"; }", shellQuote(runID), sessionDonePrefix, sessionStartPrefix, command)
}
