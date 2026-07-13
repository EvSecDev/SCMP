
export type RepoFileStatus = {
    path: string;
    status: string;
    [key: string]: unknown;
};

export type RepoStatus = {
    staged: RepoFileStatus[];
    unstaged: RepoFileStatus[];
    [key: string]: unknown;
};

export type RepoCommitInfo = {
    shortHash: string;
    fullHash: string;
    date: string;
    authorName: string;
    authorEmail: string;
    numberOfChanges: number;
    filesChanged: RepoFileStatus[];
    message: string;
    gpgSignature?: string;
    branches?: string[];
    tags?: string[];
    [key: string]: unknown;
};

export type DiffFile = {
    old_path?: string;
    new_path?: string;
    change_type: string;
    is_binary: boolean;
    hunks?: DiffHunk[];
};

export type DiffHunk = {
    old_start_line: number;
    old_line_count: number;
    new_start_line: number;
    new_line_count: number;
    changes: LineChange[];
};

export type LineChange = {
    type: "add" | "del" | "context";
    content: string;
};

export type RepoFileDiffResp = {
    files: DiffFile[];
    [key: string]: unknown;
};
