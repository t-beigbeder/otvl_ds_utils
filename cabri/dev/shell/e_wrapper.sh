timer=15

dbg() {
	if [ "$EWR_LOGFILE" ] ; then
		echo `date` "$@" >> $EWR_LOGFILE
	else
		:
	fi
}

err() {
	dbg ERR "$@"
	return 1
}

clean() {
	for pid in $* ; do
		dbg kill $pid
		kill $pid 2> /dev/null
	done
}

list_descendants() {
  children=`ps -o pid= --ppid "$1" | grep -v PID`

  for pid in $children
  do
    v=`list_descendants "$pid"`
    children="$children $v"
  done

  echo "$children"
}

monitor_parent_for() {
	parent_pid=$1
	util_pid=$2
	running="1"
	while [ "$running" ] ; do
		sleep $timer
		# dbg monitor_parent_for $parent_pid $util_pid wakeup
		kill -0 $parent_pid 2>&1 || err monitor_parent_for parent died || running=
		v=`ps -f -q $parent_pid | grep -v '^UID'`
		v=`echo $v | cut -d' ' -f3`
		desc=`list_descendants $util_pid`
		# dbg monitor_parent_for $parent_pid $util_pid pppid is $v desc are $desc
		if [ "$v" = "1" ] ; then
			err monitor_parent_for $parent_pid is orphaned, killing
			kill -1 $parent_pid 2>/dev/null
			return 0
		fi
	done
	dbg monitor_parent_for $parent_pid $util_pid wakeup killing $util_pid and $desc
	kill $util_pid $desc 2>/dev/null
}

c_wrapper() {
	dbg c_wrapper "$@"
	if [ "$EWR_LOGFILE" ] ; then
		echo >&2 "Redirecting subprocess output to $EWR_LOGFILE ($@)"
		("$@" >> $EWR_LOGFILE 2>&1) &
	else
		"$@" &
	fi
	util_pid=$!
	monitor_parent_for $$ $util_pid &
	monitor_pid=$!
	trap "clean $util_pid $monitor_pid" 1 2 3 15
	dbg c_wrapper ppid $PPID pid $$ wait $util_pid and $monitor_pid
	wait $util_pid
	dbg c_wrapper kill $monitor_pid
	kill $monitor_pid 2> /dev/null
	dbg c_wrapper finished $util_pid
}

dbg e_wrapper "$@"
c_wrapper "$@" &
dbg e_wrapper wait $!
wait $!
dbg e_wrapper finished $!