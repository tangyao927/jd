package shell

import (
	"fmt"
	"regexp"
	"strings"
)

type Options struct {
	Shell        string
	Binary       string
	Bind         string
	TrackShellCD bool
}

var validBinding = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const managementCase = "query|pin|unpin|pins|root|scan|history|config|init|completion|doctor|version|help|-h|--help"

func Init(options Options) (string, error) {
	if options.Bind == "" {
		options.Bind = "jd"
	}
	if err := ValidateBinding(options.Bind); err != nil {
		return "", err
	}
	if options.Binary == "" {
		return "", fmt.Errorf("binary path is required")
	}
	switch strings.ToLower(options.Shell) {
	case "zsh":
		return posixInit(options, true), nil
	case "bash":
		return posixInit(options, false), nil
	case "fish":
		return fishInit(options), nil
	case "powershell", "pwsh":
		return powerShellInit(options), nil
	default:
		return "", fmt.Errorf("unsupported shell %q", options.Shell)
	}
}

func ValidateBinding(binding string) error {
	if !validBinding.MatchString(binding) {
		return fmt.Errorf("invalid shell binding %q", binding)
	}
	return nil
}

func posixInit(options Options, zsh bool) string {
	binary := quotePOSIX(options.Binary)
	var builder strings.Builder
	fmt.Fprintf(&builder, "_JD_BIN=%s\n", binary)
	fmt.Fprintf(&builder, "%s() {\n", options.Bind)
	builder.WriteString("  local _jd_arg\n")
	builder.WriteString("  for _jd_arg in \"$@\"; do\n")
	builder.WriteString("    case \"$_jd_arg\" in -h|--help) \"$_JD_BIN\" \"$@\"; return $? ;; esac\n")
	builder.WriteString("  done\n")
	builder.WriteString("  case \"${1-}\" in\n")
	fmt.Fprintf(&builder, "    %s) \"$_JD_BIN\" \"$@\"; return $? ;;\n", managementCase)
	builder.WriteString("  esac\n")
	builder.WriteString("  local _jd_target _jd_status\n")
	builder.WriteString("  _jd_target=\"$(\"$_JD_BIN\" \"$@\")\"\n")
	builder.WriteString("  _jd_status=$?\n")
	builder.WriteString("  [ \"$_jd_status\" -eq 0 ] || return \"$_jd_status\"\n")
	builder.WriteString("  [ -n \"$_jd_target\" ] || return 0\n")
	builder.WriteString("  builtin cd -- \"$_jd_target\" || return $?\n")
	if !options.TrackShellCD {
		builder.WriteString("  \"$_JD_BIN\" _record \"$PWD\" >/dev/null 2>&1\n")
	}
	builder.WriteString("}\n")
	if options.TrackShellCD {
		builder.WriteString("_JD_LAST_PWD=\"$PWD\"\n")
		builder.WriteString("_jd_track_pwd() {\n")
		builder.WriteString("  [ \"$PWD\" = \"${_JD_LAST_PWD-}\" ] && return 0\n")
		builder.WriteString("  _JD_LAST_PWD=\"$PWD\"\n")
		builder.WriteString("  \"$_JD_BIN\" _record \"$PWD\" >/dev/null 2>&1\n")
		builder.WriteString("}\n")
		if zsh {
			builder.WriteString("autoload -Uz add-zsh-hook\n")
			builder.WriteString("add-zsh-hook -d chpwd _jd_track_pwd 2>/dev/null || true\n")
			builder.WriteString("add-zsh-hook chpwd _jd_track_pwd\n")
		} else {
			builder.WriteString("case \";${PROMPT_COMMAND-};\" in\n")
			builder.WriteString("  *\";_jd_track_pwd;\"*) ;;\n")
			builder.WriteString("  *) PROMPT_COMMAND=\"${PROMPT_COMMAND:+$PROMPT_COMMAND;}_jd_track_pwd\" ;;\n")
			builder.WriteString("esac\n")
		}
	}
	return builder.String()
}

func fishInit(options Options) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "set -g _JD_BIN '%s'\n", quoteFish(options.Binary))
	fmt.Fprintf(&builder, "function %s\n", options.Bind)
	builder.WriteString("  if contains -- -h $argv; or contains -- --help $argv\n")
	builder.WriteString("    command $_JD_BIN $argv\n")
	builder.WriteString("    return $status\n")
	builder.WriteString("  end\n")
	builder.WriteString("  set -l _jd_first ''\n")
	builder.WriteString("  if test (count $argv) -gt 0; set _jd_first $argv[1]; end\n")
	builder.WriteString("  switch $_jd_first\n")
	fmt.Fprintf(&builder, "    case %s\n", strings.ReplaceAll(managementCase, "|", " "))
	builder.WriteString("      command $_JD_BIN $argv\n")
	builder.WriteString("      return $status\n")
	builder.WriteString("  end\n")
	builder.WriteString("  set -l _jd_target (command $_JD_BIN $argv)\n")
	builder.WriteString("  set -l _jd_status $status\n")
	builder.WriteString("  if test $_jd_status -ne 0; return $_jd_status; end\n")
	builder.WriteString("  if test -z \"$_jd_target\"; return 0; end\n")
	builder.WriteString("  builtin cd -- \"$_jd_target\"; or return $status\n")
	if !options.TrackShellCD {
		builder.WriteString("  command $_JD_BIN _record \"$PWD\" >/dev/null 2>&1\n")
	}
	builder.WriteString("end\n")
	if options.TrackShellCD {
		builder.WriteString("set -g _JD_LAST_PWD \"$PWD\"\n")
		builder.WriteString("functions -e _jd_track_pwd 2>/dev/null\n")
		builder.WriteString("function _jd_track_pwd --on-variable PWD\n")
		builder.WriteString("  if test \"$PWD\" = \"$_JD_LAST_PWD\"; return; end\n")
		builder.WriteString("  set -g _JD_LAST_PWD \"$PWD\"\n")
		builder.WriteString("  command $_JD_BIN _record \"$PWD\" >/dev/null 2>&1\n")
		builder.WriteString("end\n")
	}
	return builder.String()
}

func powerShellInit(options Options) string {
	binary := strings.ReplaceAll(options.Binary, "'", "''")
	var builder strings.Builder
	fmt.Fprintf(&builder, "$global:__jd_bin = '%s'\n", binary)
	builder.WriteString("$global:__jd_management = @('query','pin','unpin','pins','root','scan','history','config','init','completion','doctor','version','help')\n")
	fmt.Fprintf(&builder, "function global:%s {\n", options.Bind)
	builder.WriteString("  param([Parameter(ValueFromRemainingArguments=$true)][object[]]$RemainingArgs)\n")
	builder.WriteString("  if ($RemainingArgs -contains '-h' -or $RemainingArgs -contains '--help') { & $global:__jd_bin @RemainingArgs; return }\n")
	builder.WriteString("  $first = if ($RemainingArgs.Count -gt 0) { [string]$RemainingArgs[0] } else { '' }\n")
	builder.WriteString("  if ($global:__jd_management -contains $first) { & $global:__jd_bin @RemainingArgs; return }\n")
	builder.WriteString("  $targetOutput = @(& $global:__jd_bin @RemainingArgs)\n")
	builder.WriteString("  $code = $LASTEXITCODE\n")
	builder.WriteString("  if ($code -ne 0) { $global:LASTEXITCODE = $code; return }\n")
	builder.WriteString("  $target = [string]::Join([Environment]::NewLine, $targetOutput).Trim()\n")
	builder.WriteString("  if ([string]::IsNullOrWhiteSpace($target)) { return }\n")
	builder.WriteString("  Set-Location -LiteralPath $target\n")
	if !options.TrackShellCD {
		builder.WriteString("  & $global:__jd_bin _record $PWD.Path *> $null\n")
	}
	builder.WriteString("}\n")
	if options.TrackShellCD {
		builder.WriteString("$global:__jd_last_pwd = $PWD.Path\n")
		builder.WriteString("if (-not (Get-Variable -Name __jd_original_prompt -Scope Global -ErrorAction SilentlyContinue)) {\n")
		builder.WriteString("  $global:__jd_original_prompt = if (Test-Path Function:\\prompt) { $function:prompt } else { { 'PS ' + $PWD + '> ' } }\n")
		builder.WriteString("}\n")
		builder.WriteString("function global:prompt {\n")
		builder.WriteString("  if ($PWD.Path -ne $global:__jd_last_pwd) {\n")
		builder.WriteString("    $global:__jd_last_pwd = $PWD.Path\n")
		builder.WriteString("    & $global:__jd_bin _record $PWD.Path *> $null\n")
		builder.WriteString("  }\n")
		builder.WriteString("  & $global:__jd_original_prompt\n")
		builder.WriteString("}\n")
	}
	return builder.String()
}

func quotePOSIX(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func quoteFish(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `'`, `\'`)
}
