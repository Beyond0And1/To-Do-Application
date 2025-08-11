package main

import "fmt"

type Person struct {
	first string
}

func (p *Person) speak() {
     fmt.Println("hello")
}

type human interface {
	speak()
}

func saySomething(h human) {
	h.speak()
}
func main() {

	p1 := Person{
		first: "David",
	}

	//saySomething(p1)
	saySomething(&p1)

	p1.speak()
}

/*create a type person struct
● attach a method speak to type person using a pointer receiver
○ *person
● create a type human interface
○ to implicitly implement the interface, a human must have the speak method
● create func “saySomething”
○ have it take in a human as a parameter
○ have it call the speak method
● show the following in your code
○ you CAN pass a value of type *person into saySomething
○ you CANNOT pass a value of type person into saySomething
● here is a hint if you need some help*/
