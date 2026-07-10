package gitlib

import (
	"fmt"
	"os"
	"path"
)

// TODO: fix this & add config path.
func (gr *LocalGitRepository) EnableWebHook(repoFullName string) error {
	p := path.Join(gr.GitDirectoryPath, "hooks", "update")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil { return err }
	_, err = fmt.Fprintf(f, `
#!/bin/sh

# --- Command line
refname="$1"
oldrev="$2"
newrev="$3"

# --- Safety check
if [ -z "$GIT_DIR" ]; then
	echo "Don't run this script from the command line." >&2
	echo " (if you want, you could supply GIT_DIR then run" >&2
	echo "  $0 <ref> <oldrev> <newrev>)" >&2
	exit 1
fi

if [ -z "$refname" -o -z "$oldrev" -o -z "$newrev" ]; then
	echo "usage: $0 <ref> <oldrev> <newrev>" >&2
	exit 1
fi

# --- Config
allowunannotated=$(git config --type=bool hooks.allowunannotated)
allowdeletebranch=$(git config --type=bool hooks.allowdeletebranch)
denycreatebranch=$(git config --type=bool hooks.denycreatebranch)
allowdeletetag=$(git config --type=bool hooks.allowdeletetag)
allowmodifytag=$(git config --type=bool hooks.allowmodifytag)

# --- Check types
# if $newrev is 0000...0000, it's a commit to delete a ref.
zero=$(git hash-object --stdin </dev/null | tr '[0-9a-f]' '0')
if [ "$newrev" = "$zero" ]; then
	newrev_type=delete
else
	newrev_type=$(git cat-file -t $newrev)
fi

  case "$refname","$newrev_type" in
	refs/tags/*,commit)
		# un-annotated tag
		short_refname=${refname##refs/tags/}
		if [ "$allowunannotated" != "true" ]; then
			echo "*** The un-annotated tag, $short_refname, is not allowed in this repository" >&2
			echo "*** Use 'git tag [ -a | -s ]' for tags you want to propagate." >&2
			exit 1
	    else
			gitus web-hooks send "%s" "$refname" "$newrev_type" "$oldrev" "$newrev"
		fi
		;;
	refs/tags/*,delete)
		# delete tag
		if [ "$allowdeletetag" != "true" ]; then
			echo "*** Deleting a tag is not allowed in this repository" >&2
			exit 1
		else
			gitus web-hooks send "%s" "$refname" "$newrev_type" "$oldrev" "$newrev"
		fi
		;;
	refs/tags/*,tag)
		# annotated tag
		if [ "$allowmodifytag" != "true" ] && git rev-parse $refname > /dev/null 2>&1
		then
			echo "*** Tag '$refname' already exists." >&2
			echo "*** Modifying a tag is not allowed in this repository." >&2
			exit 1
		else
			gitus web-hooks send "%s" "$refname" "$newrev_type" "$oldrev"  "$newrev"
		fi
		;;
	refs/heads/*,commit)
		# branch
		if [ "$oldrev" = "$zero" -a "$denycreatebranch" = "true" ]; then
			echo "*** Creating a branch is not allowed in this repository" >&2
			exit 1
		else
			gitus web-hooks send "%s" "$refname" "$newrev_type" "$oldrev"  "$newrev"
		fi
		;;	refs/heads/*,delete)
		# delete branch
		if [ "$allowdeletebranch" != "true" ]; then
			echo "*** Deleting a branch is not allowed in this repository" >&2
			exit 1
		else
			gitus web-hooks send "%s" "$refname" "$newrev_type" "$oldrev"  "$newrev"
		fi
		;;
	refs/remotes/*,commit)
		# tracking branch
		;;
	refs/remotes/*,delete)
		# delete tracking branch
		if [ "$allowdeletebranch" != "true" ]; then
			echo "*** Deleting a tracking branch is not allowed in this repository" >&2
			exit 1
		fi
		;;
	,*)
		# Anything else (is there anything else?)
		echo "*** Update hook: unknown type of update to ref $refname of type $newrev_type" >&2
		exit 1
		;;
esac

# --- Finished
exit 0
`, repoFullName, repoFullName, repoFullName, repoFullName, repoFullName)
	if err != nil { return err }
	err = f.Sync()
	if err != nil { return err }
	err = f.Close()
	if err != nil { return err }
	fi, err := os.Stat(p)
	if err != nil { return err }
	err = os.Chmod(p, os.FileMode(uint32(fi.Mode())|0100))
	if err != nil { return err }
	return nil
}

func (gr *LocalGitRepository) DisableWebHook() error {
	p := path.Join(gr.GitDirectoryPath, "hooks", "update")
	err := os.RemoveAll(p)
	if err != nil { return err }
	return nil
}

