package model

type InlinePicture struct {
	Filepath  string
	ContentId string
}

type ActivationMailTemplate struct {
	TemplateFile   string
	InlinePictures []InlinePicture
}
