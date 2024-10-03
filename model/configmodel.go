package model

type InlinePicture struct {
	Filepath  string
	ContentId string
}

type FileAttachment struct {
	Name string
	Mime string
}

type ActivationMailTemplate struct {
	TemplateFile   string
	InlinePictures []InlinePicture
	Attachment     FileAttachment
}
