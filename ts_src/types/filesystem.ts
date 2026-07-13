
export type FileMetadata = {
    path: string;
    type: string;
    size: number;
    ownerName: string;
    groupName: string;
    permissions: string;
    lastModified?: string;
    externalContentLocation?: string;
    symbolicLinkTarget?: string;
    dependencies?: string[];
    preDeployCommands?: string[];
    installCommands?: string[];
    postInstallCommands?: string[];
    preApplyCommands?: string[];
    postApplyCommands?: string[];
    reloadCommands?: string[];
    reloadGroup?: string;
    [key: string]: unknown;
};

export type DownloadLink = {
    downloadLocation: string;
    [key: string]: unknown;
};

export type FileOp = {
    path: string;
    type: string;
    recursive: boolean;
};

export type FileMove = {
    sourcePath: string;
    destinationPath: string;
    deleteSource: boolean;
    overwriteDestination: boolean;
};

export type FilePathSearchReq = {
    path: string;
    query: string;
    searchType: string;
    fileType: string;
    depth: number;
};

export type FilePathSearchResults = {
    orig: FilePathSearchReq;
    matchCount: number;
    matches: FileMetadata[];
    [key: string]: unknown;
};

export type PathList = {
    paths: string[];
};
