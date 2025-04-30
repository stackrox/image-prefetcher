function info()
{
  echo >&2 "$(date --iso-8601=seconds)" "$@"
}
